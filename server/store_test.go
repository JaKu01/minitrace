package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreKeepsOnlyConfiguredRetention(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "test.db"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	spans := []Span{
		{
			TraceID: "fresh-trace", SpanID: "fresh-span", ServiceName: "api", Name: "fresh",
			StartTime: now.Add(-time.Minute), EndTime: now, Status: "ok",
			Attributes: json.RawMessage(`{}`), Events: json.RawMessage(`[]`),
		},
		{
			TraceID: "old-trace", SpanID: "old-span", ServiceName: "api", Name: "old",
			StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-2*time.Hour + time.Second), Status: "ok",
		},
	}
	inserted, err := store.InsertSpans(context.Background(), spans)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 {
		t.Fatalf("inserted %d spans, want 1", inserted)
	}
	traces, err := store.ListTraces(context.Background(), "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || traces[0].TraceID != "fresh-trace" {
		t.Fatalf("unexpected traces: %#v", traces)
	}
}

func TestRetentionCannotExceedOneDay(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "test.db"), 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.Retention() != 24*time.Hour {
		t.Fatalf("retention is %s, want 24h", store.Retention())
	}
}
