package database

import (
	"context"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// SeededMockDB embeds MockDB and overrides QueryRow/Query to return
// pre-populated auth data so Keystone token issuance works without a
// real database. All other queries fall through to MockDB behaviour
// (ErrNoRows / empty rows).
type SeededMockDB struct {
	*MockDB
	adminHash string // bcrypt hash of "secret"
}

// NewSeededMockDB constructs a SeededMockDB with a bcrypt hash of "secret"
// pre-generated at MinCost for test speed.
func NewSeededMockDB() *SeededMockDB {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		// bcrypt.GenerateFromPassword only fails on invalid cost; MinCost is always valid.
		panic("seeded mock: bcrypt failed: " + err.Error())
	}
	return &SeededMockDB{
		MockDB:    NewMockDB(),
		adminHash: string(hash),
	}
}

// QueryRow overrides MockDB.QueryRow to return seeded rows for the three auth
// queries Keystone runs during token issuance. Anything else returns ErrNoRows.
func (s *SeededMockDB) QueryRow(ctx context.Context, sql string, args ...any) Row {
	switch {
	// Domain lookup: SELECT id FROM domains WHERE name = $1
	case strings.Contains(sql, "FROM domains") && strings.Contains(sql, "WHERE name"):
		return &seededRow{values: []any{"default-domain-id"}}

	// Domain lookup by ID: SELECT name FROM domains WHERE id = $1  (used in response building)
	case strings.Contains(sql, "FROM domains") && strings.Contains(sql, "WHERE id"):
		return &seededRow{values: []any{"Default"}}

	// User lookup: SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1
	case strings.Contains(sql, "FROM users") && strings.Contains(sql, "WHERE name"):
		return &seededRow{values: []any{
			"admin-user-id", "admin", s.adminHash, true, "default-domain-id",
		}}

	// User lookup by ID: SELECT id, name, password_hash, enabled, domain_id FROM users WHERE id = $1
	case strings.Contains(sql, "FROM users") && strings.Contains(sql, "WHERE id"):
		return &seededRow{values: []any{
			"admin-user-id", "admin", s.adminHash, true, "default-domain-id",
		}}

	// Project lookup: SELECT id, name, description, enabled, domain_id FROM projects WHERE name/id = ...
	case strings.Contains(sql, "FROM projects"):
		return &seededRow{values: []any{
			"default-project-id", "default", "", true, "default-domain-id",
		}}
	}

	return &mockRow{err: ErrNoRows}
}

// Query overrides MockDB.Query to return role names for role_assignments queries
// and empty rows for everything else (triggering hardcoded catalog fallback).
func (s *SeededMockDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if strings.Contains(sql, "role_assignments") || strings.Contains(sql, "FROM roles") {
		return &seededRoles{names: []string{"admin", "member"}, idx: -1}, nil
	}
	return &mockRows{}, nil
}

// seededRow implements Row, filling Scan destinations from a pre-set value
// slice in declaration order. Supports *string, *bool, and *int destinations.
type seededRow struct {
	values []any
}

func (r *seededRow) Scan(dest ...any) error {
	for i, d := range dest {
		if i >= len(r.values) {
			break
		}
		v := r.values[i]
		switch dst := d.(type) {
		case *string:
			if s, ok := v.(string); ok {
				*dst = s
			}
		case *bool:
			if b, ok := v.(bool); ok {
				*dst = b
			}
		case *int:
			if n, ok := v.(int); ok {
				*dst = n
			}
		}
	}
	return nil
}

// seededRoles implements Rows and iterates over a fixed list of role name
// strings, each yielded by a single-string Scan call.
type seededRoles struct {
	names []string
	idx   int
}

func (r *seededRoles) Next() bool { r.idx++; return r.idx < len(r.names) }
func (r *seededRoles) Close()     {}
func (r *seededRoles) Err() error { return nil }

func (r *seededRoles) Scan(dest ...any) error {
	if r.idx < 0 || r.idx >= len(r.names) {
		return ErrNoRows
	}
	if len(dest) > 0 {
		if dst, ok := dest[0].(*string); ok {
			*dst = r.names[r.idx]
		}
	}
	return nil
}
