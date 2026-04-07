package common

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParsePagination_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/test", nil)
	p := ParsePagination(c, 1000)
	if p.Limit != 1000 {
		t.Errorf("Expected default limit 1000, got %d", p.Limit)
	}
	if p.Offset != 0 {
		t.Errorf("Expected default offset 0, got %d", p.Offset)
	}
	if p.Marker != "" {
		t.Errorf("Expected empty marker, got %q", p.Marker)
	}
}

func TestParsePagination_CustomValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/test?limit=50&offset=10&marker=abc-123", nil)
	p := ParsePagination(c, 1000)
	if p.Limit != 50 {
		t.Errorf("Expected limit 50, got %d", p.Limit)
	}
	if p.Offset != 10 {
		t.Errorf("Expected offset 10, got %d", p.Offset)
	}
	if p.Marker != "abc-123" {
		t.Errorf("Expected marker 'abc-123', got %q", p.Marker)
	}
}

func TestParsePagination_InvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/test?limit=-5", nil)
	p := ParsePagination(c, 1000)
	if p.Limit != 1000 {
		t.Errorf("Expected default limit for invalid value, got %d", p.Limit)
	}
}

func TestParsePagination_SortKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/test?sort_key=name&sort_dir=desc", nil)
	p := ParsePagination(c, 1000)
	if p.SortKey != "name" {
		t.Errorf("Expected sort_key 'name', got %q", p.SortKey)
	}
	if p.SortDir != "desc" {
		t.Errorf("Expected sort_dir 'desc', got %q", p.SortDir)
	}
}
