package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestIngestAndReadTrace(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "api.db"), MaxRetention)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := New(store, nil, nil).Handler()

	now := time.Now().UTC()
	body, err := json.Marshal(map[string]any{"spans": []Span{{
		TraceID: "trace-1", SpanID: "span-1", ServiceName: "checkout",
		Name: "POST /orders", StartTime: now.Add(-time.Millisecond),
		EndTime: now, Status: "ok",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	ingest := httptest.NewRequest(http.MethodPost, "/api/v1/spans", bytes.NewReader(body))
	ingest.Header.Set("Content-Type", "application/json")
	ingestResponse := httptest.NewRecorder()
	handler.ServeHTTP(ingestResponse, ingest)
	if ingestResponse.Code != http.StatusAccepted {
		t.Fatalf("ingest status %d: %s", ingestResponse.Code, ingestResponse.Body.String())
	}

	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var traces []TraceSummary
	if err := json.NewDecoder(listResponse.Body).Decode(&traces); err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || traces[0].TraceID != "trace-1" {
		t.Fatalf("unexpected traces: %#v", traces)
	}

	detailResponse := httptest.NewRecorder()
	handler.ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1", nil))
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", detailResponse.Code, detailResponse.Body.String())
	}
}

func TestServerServesAngularAppAndSPAFallback(t *testing.T) {
	app := fstest.MapFS{
		"index.html": {Data: []byte("<html>minitrace app</html>")},
		"main.js":    {Data: []byte("console.log('minitrace')")},
	}
	handler := New(nil, nil, app).Handler()

	for _, path := range []string{"/", "/trace/example"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned status %d", path, response.Code)
		}
		if !bytes.Contains(response.Body.Bytes(), []byte("minitrace app")) {
			t.Fatalf("%s did not serve Angular index", path)
		}
	}
}
