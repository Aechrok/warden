package debug

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxEntries = 200

type contextKey struct{}

type traceStart struct {
	sql   string
	start time.Time
}

// QueryEntry holds a single recorded DB query.
type QueryEntry struct {
	SQL        string  `json:"sql"`
	DurationMs float64 `json:"duration_ms"`
	OK         bool    `json:"ok"`
	At         string  `json:"at"`
}

// Tracer implements pgx.QueryTracer and records queries in an in-memory ring buffer.
type Tracer struct {
	mu      sync.Mutex
	entries []QueryEntry
	total   int64
	sumMs   float64
}

func (t *Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	sql := data.SQL
	if len(sql) > 150 {
		sql = sql[:150] + "…"
	}
	return context.WithValue(ctx, contextKey{}, traceStart{sql: sql, start: time.Now()})
}

func (t *Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	v, ok := ctx.Value(contextKey{}).(traceStart)
	if !ok {
		return
	}
	dur := float64(time.Since(v.start).Microseconds()) / 1000.0
	entry := QueryEntry{
		SQL:        v.sql,
		DurationMs: dur,
		OK:         data.Err == nil,
		At:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, entry)
	if len(t.entries) > maxEntries {
		t.entries = t.entries[1:]
	}
	t.total++
	t.sumMs += dur
}

// Stats is the snapshot returned to the debug HTTP handler.
type Stats struct {
	QueryCount int64        `json:"query_count"`
	AvgMs      float64      `json:"avg_ms"`
	Queries    []QueryEntry `json:"queries"`
}

func (t *Tracer) Stats() Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	var avg float64
	if t.total > 0 {
		avg = t.sumMs / float64(t.total)
	}
	// Reverse so newest entry is first.
	out := make([]QueryEntry, len(t.entries))
	for i, e := range t.entries {
		out[len(t.entries)-1-i] = e
	}
	return Stats{QueryCount: t.total, AvgMs: avg, Queries: out}
}
