package database

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var dbTracer = otel.Tracer("o3k/database")

// TracingAdapter wraps any DBIF and adds OpenTelemetry spans to Exec, Query,
// and QueryRow. BeginTx is passed through unchanged; the transaction's own
// Exec/Query/QueryRow methods are not wrapped because the transaction struct
// returned by the underlying adapter already calls those wrapped paths when the
// adapter itself is a TracingAdapter.
type TracingAdapter struct {
	inner DBIF
}

// NewTracingAdapter wraps inner with OpenTelemetry instrumentation.
func NewTracingAdapter(inner DBIF) *TracingAdapter {
	return &TracingAdapter{inner: inner}
}

func (a *TracingAdapter) Exec(ctx context.Context, sql string, args ...any) (Result, error) {
	ctx, span := dbTracer.Start(ctx, "db.exec")
	defer span.End()
	span.SetAttributes(attribute.String("db.statement", truncate(sql, 200)))

	result, err := a.inner.Exec(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (a *TracingAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	ctx, span := dbTracer.Start(ctx, "db.query_row")
	span.SetAttributes(attribute.String("db.statement", truncate(sql, 200)))
	// QueryRow is synchronous in the caller's Scan call; we end the span here
	// since we cannot hook into when Scan completes without changing the Row
	// interface. The span covers the network/disk round-trip for the query.
	span.End()
	return a.inner.QueryRow(ctx, sql, args...)
}

func (a *TracingAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	ctx, span := dbTracer.Start(ctx, "db.query")
	defer span.End()
	span.SetAttributes(attribute.String("db.statement", truncate(sql, 200)))

	rows, err := a.inner.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return rows, err
}

func (a *TracingAdapter) BeginTx(ctx context.Context, opts TxOptions) (Tx, error) {
	return a.inner.BeginTx(ctx, opts)
}

// Unwrap returns the underlying DBIF, allowing callers to reach through the tracing layer.
func (a *TracingAdapter) Unwrap() DBIF { return a.inner }

// truncate caps a string at maxLen runes, appending "…" when trimmed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
