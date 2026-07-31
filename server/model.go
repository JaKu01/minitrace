package server

import (
	"encoding/json"
	"time"
)

type Span struct {
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId,omitempty"`
	ServiceName  string          `json:"serviceName"`
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	EndTime      time.Time       `json:"endTime"`
	Status       string          `json:"status"`
	Attributes   json.RawMessage `json:"attributes,omitempty"`
	Events       json.RawMessage `json:"events,omitempty"`
}

type TraceSummary struct {
	TraceID      string    `json:"traceId"`
	Name         string    `json:"name"`
	ServiceName  string    `json:"serviceName"`
	StartTime    time.Time `json:"startTime"`
	DurationNano int64     `json:"durationNano"`
	SpanCount    int       `json:"spanCount"`
	Status       string    `json:"status"`
}

type TraceDetail struct {
	TraceID      string `json:"traceId"`
	DurationNano int64  `json:"durationNano"`
	Spans        []Span `json:"spans"`
}
