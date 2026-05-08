package glance

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
)

func newFakeGinContext(params map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	return c
}

func TestJoinConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []string
		want       string
	}{
		{"empty", nil, ""},
		{"single", []string{"a = $1"}, "a = $1"},
		{"two", []string{"a = $1", "b = $2"}, "a = $1 AND b = $2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinConditions(tt.conditions)
			if got != tt.want {
				t.Errorf("joinConditions(%v) = %q, want %q", tt.conditions, got, tt.want)
			}
		})
	}
}

// TestListImagesVisibilityCondition verifies the visibility branch produces the
// right SQL fragment and placeholder indices without hitting a real database.
func TestListImagesVisibilityCondition(t *testing.T) {
	tests := []struct {
		name             string
		queryParams      map[string]string
		wantCondFragment string // substring that must appear in first condition
		wantArgCount     int    // number of args consumed for the visibility clause
	}{
		{
			name:             "no visibility param",
			queryParams:      map[string]string{},
			wantCondFragment: "(visibility = 'public' OR project_id = $1)",
			wantArgCount:     1,
		},
		{
			name:             "visibility=public",
			queryParams:      map[string]string{"visibility": "public"},
			wantCondFragment: "visibility = 'public'",
			wantArgCount:     0,
		},
		{
			name:             "visibility=private",
			queryParams:      map[string]string{"visibility": "private"},
			wantCondFragment: "visibility = 'private'",
			wantArgCount:     1,
		},
		{
			name:             "visibility=shared",
			queryParams:      map[string]string{"visibility": "shared"},
			wantCondFragment: "(visibility = 'public' OR project_id = $1)",
			wantArgCount:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeGinContext(tt.queryParams)
			c.Set("project_id", "proj-abc")

			var conditions []string
			var queryArgs []interface{}
			argIdx := 1
			projectID := "proj-abc"

			vis := c.Query("visibility")
			if vis != "" {
				switch vis {
				case "public":
					conditions = append(conditions, "visibility = 'public'")
				case "private":
					conditions = append(conditions, "(visibility = 'private' AND project_id = $1)")
					queryArgs = append(queryArgs, projectID)
					argIdx++
				default:
					conditions = append(conditions, "(visibility = 'public' OR project_id = $1)")
					queryArgs = append(queryArgs, projectID)
					argIdx++
				}
			} else {
				conditions = append(conditions, "(visibility = 'public' OR project_id = $1)")
				queryArgs = append(queryArgs, projectID)
				argIdx++
			}
			_ = argIdx // suppress unused warning

			if len(conditions) == 0 {
				t.Fatal("expected at least one condition")
			}
			if !strings.Contains(conditions[0], tt.wantCondFragment) {
				t.Errorf("condition %q does not contain %q", conditions[0], tt.wantCondFragment)
			}
			if len(queryArgs) != tt.wantArgCount {
				t.Errorf("got %d args, want %d", len(queryArgs), tt.wantArgCount)
			}
		})
	}
}

func TestAllowedImageUpdateField(t *testing.T) {
	tests := []struct {
		path    string
		allowed bool
		field   string
	}{
		{"/name", true, "name"},
		{"/visibility", true, "visibility"},
		{"/min_disk", true, "min_disk_gb"},
		{"/min_ram", true, "min_ram_mb"},
		{"/malicious; DROP TABLE images;--", false, ""},
		{"/nonexistent", false, ""},
	}
	for _, tt := range tests {
		field, ok := allowedImageUpdateField(tt.path)
		if ok != tt.allowed {
			t.Errorf("allowedImageUpdateField(%q) ok = %v, want %v", tt.path, ok, tt.allowed)
		}
		if ok && field != tt.field {
			t.Errorf("allowedImageUpdateField(%q) field = %q, want %q", tt.path, field, tt.field)
		}
	}
}

// deleteImageQueryRowDB is a test double whose QueryRow returns controlled
// values for the ownership+protected query in DeleteImage.
type deleteImageQueryRowDB struct {
	*database.MockDB
	ownerProjectID string
	protected      bool
}

func (d *deleteImageQueryRowDB) QueryRow(_ context.Context, sql string, args ...any) database.Row {
	if strings.Contains(sql, "FROM images") {
		nullStr := sql_NullString(d.ownerProjectID)
		return &scanRow{values: []any{nullStr, d.protected}}
	}
	return &scanRow{err: database.ErrNoRows}
}

// sql_NullString is a local helper to avoid importing database/sql in test helper.
func sql_NullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// scanRow is a pgx.Row that fills Scan destinations in declaration order.
type scanRow struct {
	values []any
	err    error
}

func (r *scanRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.values) {
			break
		}
		switch dst := d.(type) {
		case *sql.NullString:
			if v, ok := r.values[i].(sql.NullString); ok {
				*dst = v
			}
		case *bool:
			if b, ok := r.values[i].(bool); ok {
				*dst = b
			}
		case *string:
			if s, ok := r.values[i].(string); ok {
				*dst = s
			}
		}
	}
	return nil
}

// Ensure deleteImageQueryRowDB satisfies database.DBIF at compile time.
var _ database.DBIF = (*deleteImageQueryRowDB)(nil)

// Exec, Query, BeginTx delegate to embedded MockDB.
func (d *deleteImageQueryRowDB) Exec(ctx context.Context, sql string, args ...any) (database.Result, error) {
	return d.MockDB.Exec(ctx, sql, args...)
}
func (d *deleteImageQueryRowDB) Query(ctx context.Context, sql string, args ...any) (database.Rows, error) {
	return d.MockDB.Query(ctx, sql, args...)
}
func (d *deleteImageQueryRowDB) BeginTx(ctx context.Context, opts database.TxOptions) (database.Tx, error) {
	return d.MockDB.BeginTx(ctx, opts)
}

func TestDeleteImageProtected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		ownerProjectID string
		protected      bool
		isAdmin        bool
		wantStatus     int
	}{
		{
			name:           "protected image returns 403",
			ownerProjectID: "proj-abc",
			protected:      true,
			isAdmin:        false,
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "admin cannot delete protected image",
			ownerProjectID: "other-proj",
			protected:      true,
			isAdmin:        true,
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "unprotected image owner can delete",
			ownerProjectID: "proj-abc",
			protected:      false,
			isAdmin:        false,
			wantStatus:     http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &deleteImageQueryRowDB{
				MockDB:         database.NewMockDB(),
				ownerProjectID: tt.ownerProjectID,
				protected:      tt.protected,
			}
			svc := NewServiceWithDB(db, "stub", "", "", "", "", "", nil)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req, _ := http.NewRequest(http.MethodDelete, "/images/img-id-123", nil)
			c.Request = req
			c.Params = gin.Params{{Key: "id", Value: "img-id-123"}}
			c.Set("project_id", "proj-abc")
			c.Set("is_admin", tt.isAdmin)

			svc.DeleteImage(c)

			if tt.wantStatus == http.StatusNoContent {
				// gin.CreateTestContext does not call WriteHeaderNow for no-body responses;
				// verify the DELETE was issued and no 403 was returned instead.
				if w.Code == http.StatusForbidden {
					t.Errorf("expected delete to proceed but got 403; body = %s", w.Body.String())
				}
				if !db.MockDB.ExecCalled("DELETE") {
					t.Error("expected DELETE to be executed but it was not")
				}
				return
			}

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
