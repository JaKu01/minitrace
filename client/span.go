package minitrace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type spanContextKey struct{}

// SpanData is the wire representation sent to the minitrace server.
type SpanData struct {
	TraceID      string         `json:"traceId"`
	SpanID       string         `json:"spanId"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	ServiceName  string         `json:"serviceName"`
	Name         string         `json:"name"`
	StartTime    time.Time      `json:"startTime"`
	EndTime      time.Time      `json:"endTime"`
	Status       string         `json:"status"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []Event        `json:"events,omitempty"`
}

type Event struct {
	Name       string         `json:"name"`
	Time       time.Time      `json:"time"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Span records one timed operation. End is safe to call more than once.
type Span struct {
	tracer *Tracer
	data   SpanData
	once   sync.Once
	mu     sync.Mutex
}

func (s *Span) SetAttribute(key string, value any) *Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Attributes == nil {
		s.data.Attributes = make(map[string]any)
	}
	s.data.Attributes[key] = value
	return s
}

func (s *Span) AddEvent(name string, attributes map[string]any) *Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Events = append(s.data.Events, Event{
		Name:       name,
		Time:       time.Now().UTC(),
		Attributes: attributes,
	})
	return s
}

func (s *Span) SetError(err error) *Span {
	if err == nil {
		return s
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Status = "error"
	if s.data.Attributes == nil {
		s.data.Attributes = make(map[string]any)
	}
	s.data.Attributes["error.message"] = err.Error()
	return s
}

func (s *Span) End() {
	s.once.Do(func() {
		s.mu.Lock()
		s.data.EndTime = time.Now().UTC()
		data := s.data
		s.mu.Unlock()
		s.tracer.enqueue(data)
	})
}

func spanFromContext(ctx context.Context) *Span {
	span, _ := ctx.Value(spanContextKey{}).(*Span)
	return span
}

func newID(bytes int) string {
	id := make([]byte, bytes)
	if _, err := rand.Read(id); err != nil {
		panic(fmt.Sprintf("minitrace: generate id: %v", err))
	}
	return hex.EncodeToString(id)
}
