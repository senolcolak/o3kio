package common

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	// MaxPaginationLimit is the hard ceiling for any list endpoint.
	MaxPaginationLimit = 1000
	// DefaultPaginationLimit is used when no limit is provided.
	DefaultPaginationLimit = 25
)

// PaginationParams holds the parsed pagination query parameters from a request.
type PaginationParams struct {
	Limit   int
	Offset  int
	Marker  string
	SortKey string
	SortDir string
}

// CapLimit enforces pagination bounds on a raw limit value.
// If limit <= 0 it returns DefaultPaginationLimit.
// If limit > MaxPaginationLimit it returns MaxPaginationLimit.
func CapLimit(limit int) int {
	if limit <= 0 {
		return DefaultPaginationLimit
	}
	if limit > MaxPaginationLimit {
		return MaxPaginationLimit
	}
	return limit
}

// ParsePagination extracts limit, offset, marker, sort_key, and sort_dir from
// the request query string. defaultLimit is used when the caller does not
// supply a limit (or supplies an invalid one).
func ParsePagination(c *gin.Context, defaultLimit int) PaginationParams {
	p := PaginationParams{
		Limit:   defaultLimit,
		Marker:  c.Query("marker"),
		SortKey: c.DefaultQuery("sort_key", "created_at"),
		SortDir: c.DefaultQuery("sort_dir", "desc"),
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			p.Limit = v
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			p.Offset = v
		}
	}

	if p.SortDir != "asc" && p.SortDir != "desc" {
		p.SortDir = "desc"
	}

	// Enforce hard cap on limit.
	p.Limit = CapLimit(p.Limit)

	return p
}
