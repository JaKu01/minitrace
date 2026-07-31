package minitrace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTracerExportsParentAndChildAsBatch(t *testing.T) {
	received := make(chan spanBatch, 1)
	httpClient := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var batch spanBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Errorf("decode batch: %v", err)
		}
		received <- batch
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}

	tracer, err := New(Config{
		ServiceName:   "checkout",
		Endpoint:      "http://minitrace.test",
		HTTPClient:    httpClient,
		FlushInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, parent := tracer.Start(context.Background(), "request")
	_, child := tracer.Start(ctx, "database")
	child.End()
	parent.End()

	select {
	case batch := <-received:
		if len(batch.Spans) != 2 {
			t.Fatalf("got %d spans, want 2", len(batch.Spans))
		}
		if batch.Spans[0].TraceID != batch.Spans[1].TraceID {
			t.Fatal("parent and child have different trace ids")
		}
		if batch.Spans[0].ParentSpanID != batch.Spans[1].SpanID {
			t.Fatal("child does not reference parent span")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for exported spans")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tracer.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
}
