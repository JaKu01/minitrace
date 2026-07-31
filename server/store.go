package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const MaxRetention = 24 * time.Hour

var ErrTraceNotFound = errors.New("trace not found")

type Store struct {
	db        *sql.DB
	retention time.Duration
}

func OpenStore(path string, retention time.Duration) (*Store, error) {
	retention = normalizeRetention(retention)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db, retention: retention}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.DeleteExpired(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func normalizeRetention(retention time.Duration) time.Duration {
	if retention <= 0 || retention > MaxRetention {
		return MaxRetention
	}
	return retention
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Retention() time.Duration {
	return s.retention
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
CREATE TABLE IF NOT EXISTS spans (
	trace_id TEXT NOT NULL,
	span_id TEXT PRIMARY KEY,
	parent_span_id TEXT NOT NULL DEFAULT '',
	service_name TEXT NOT NULL,
	name TEXT NOT NULL,
	start_time INTEGER NOT NULL,
	end_time INTEGER NOT NULL,
	status TEXT NOT NULL,
	attributes_json TEXT NOT NULL DEFAULT '{}',
	events_json TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_spans_trace_start ON spans(trace_id, start_time);
CREATE INDEX IF NOT EXISTS idx_spans_start ON spans(start_time);
CREATE INDEX IF NOT EXISTS idx_spans_service_start ON spans(service_name, start_time);
PRAGMA optimize;
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) InsertSpans(ctx context.Context, spans []Span) (int, error) {
	if len(spans) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	statement, err := tx.PrepareContext(ctx, `
INSERT INTO spans (
	trace_id, span_id, parent_span_id, service_name, name,
	start_time, end_time, status, attributes_json, events_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(span_id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	cutoff := time.Now().Add(-s.retention)
	inserted := 0
	for _, span := range spans {
		if err := validateSpan(span); err != nil {
			return 0, err
		}
		if span.EndTime.Before(cutoff) {
			continue
		}
		attributes := normalizedJSON(span.Attributes, `{}`)
		events := normalizedJSON(span.Events, `[]`)
		result, err := statement.ExecContext(
			ctx,
			span.TraceID,
			span.SpanID,
			span.ParentSpanID,
			span.ServiceName,
			span.Name,
			span.StartTime.UnixNano(),
			span.EndTime.UnixNano(),
			span.Status,
			attributes,
			events,
		)
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		inserted += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func validateSpan(span Span) error {
	switch {
	case span.TraceID == "":
		return errors.New("traceId is required")
	case span.SpanID == "":
		return errors.New("spanId is required")
	case strings.TrimSpace(span.ServiceName) == "":
		return errors.New("serviceName is required")
	case strings.TrimSpace(span.Name) == "":
		return errors.New("name is required")
	case span.StartTime.IsZero() || span.EndTime.IsZero():
		return errors.New("startTime and endTime are required")
	case span.EndTime.Before(span.StartTime):
		return errors.New("endTime must not be before startTime")
	}
	return nil
}

func normalizedJSON(value json.RawMessage, fallback string) string {
	if len(value) == 0 || !json.Valid(value) {
		return fallback
	}
	return string(value)
}

func (s *Store) ListTraces(ctx context.Context, service, query, status string, limit int) ([]TraceSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cutoff := time.Now().Add(-s.retention).UnixNano()
	where := []string{"start_time >= ?"}
	args := []any{cutoff}
	if service != "" {
		where = append(where, "service_name = ?")
		args = append(args, service)
	}
	if query != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+query+"%")
	}
	if status == "error" {
		where = append(where, "status = 'error'")
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `
SELECT
	trace_id,
	COALESCE(MAX(CASE WHEN parent_span_id = '' THEN name END), MAX(name)),
	COALESCE(MAX(CASE WHEN parent_span_id = '' THEN service_name END), MAX(service_name)),
	MIN(start_time),
	MAX(end_time) - MIN(start_time),
	COUNT(*),
	CASE WHEN MAX(CASE WHEN status = 'error' THEN 1 ELSE 0 END) = 1 THEN 'error' ELSE 'ok' END
FROM spans
WHERE `+strings.Join(where, " AND ")+`
GROUP BY trace_id
ORDER BY MIN(start_time) DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	traces := make([]TraceSummary, 0)
	for rows.Next() {
		var trace TraceSummary
		var startNano int64
		if err := rows.Scan(
			&trace.TraceID,
			&trace.Name,
			&trace.ServiceName,
			&startNano,
			&trace.DurationNano,
			&trace.SpanCount,
			&trace.Status,
		); err != nil {
			return nil, err
		}
		trace.StartTime = time.Unix(0, startNano).UTC()
		traces = append(traces, trace)
	}
	return traces, rows.Err()
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (TraceDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT trace_id, span_id, parent_span_id, service_name, name,
       start_time, end_time, status, attributes_json, events_json
FROM spans
WHERE trace_id = ? AND start_time >= ?
ORDER BY start_time ASC`, traceID, time.Now().Add(-s.retention).UnixNano())
	if err != nil {
		return TraceDetail{}, err
	}
	defer rows.Close()

	detail := TraceDetail{TraceID: traceID, Spans: make([]Span, 0)}
	var earliest, latest int64
	for rows.Next() {
		var span Span
		var startNano, endNano int64
		var attributes, events string
		if err := rows.Scan(
			&span.TraceID,
			&span.SpanID,
			&span.ParentSpanID,
			&span.ServiceName,
			&span.Name,
			&startNano,
			&endNano,
			&span.Status,
			&attributes,
			&events,
		); err != nil {
			return TraceDetail{}, err
		}
		span.StartTime = time.Unix(0, startNano).UTC()
		span.EndTime = time.Unix(0, endNano).UTC()
		span.Attributes = json.RawMessage(attributes)
		span.Events = json.RawMessage(events)
		if earliest == 0 || startNano < earliest {
			earliest = startNano
		}
		if endNano > latest {
			latest = endNano
		}
		detail.Spans = append(detail.Spans, span)
	}
	if err := rows.Err(); err != nil {
		return TraceDetail{}, err
	}
	if len(detail.Spans) == 0 {
		return TraceDetail{}, ErrTraceNotFound
	}
	detail.DurationNano = latest - earliest
	return detail, nil
}

func (s *Store) Services(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT service_name
FROM spans
WHERE start_time >= ?
ORDER BY service_name`, time.Now().Add(-s.retention).UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	services := make([]string, 0)
	for rows.Next() {
		var service string
		if err := rows.Scan(&service); err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	return services, rows.Err()
}

func (s *Store) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM spans WHERE end_time < ?", time.Now().Add(-s.retention).UnixNano())
	return err
}
