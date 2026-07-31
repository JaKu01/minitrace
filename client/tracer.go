package minitrace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrServiceNameRequired = errors.New("minitrace: service name is required")

type spanBatch struct {
	Spans []SpanData `json:"spans"`
}

// Tracer creates spans and asynchronously exports them in bounded batches.
type Tracer struct {
	config    Config
	queue     chan SpanData
	stop      chan struct{}
	done      chan struct{}
	closed    atomic.Bool
	dropped   atomic.Uint64
	once      sync.Once
	enqueueMu sync.RWMutex
}

func New(config Config) (*Tracer, error) {
	config = config.withDefaults()
	if strings.TrimSpace(config.ServiceName) == "" {
		return nil, ErrServiceNameRequired
	}
	tracer := &Tracer{
		config: config,
		queue:  make(chan SpanData, config.QueueSize),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go tracer.exportLoop()
	return tracer, nil
}

func (t *Tracer) Start(ctx context.Context, name string) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}

	traceID := newID(16)
	parentID := ""
	if parent := spanFromContext(ctx); parent != nil {
		parent.mu.Lock()
		traceID = parent.data.TraceID
		parentID = parent.data.SpanID
		parent.mu.Unlock()
	} else if remote, ok := remoteContextFromContext(ctx); ok {
		traceID = remote.TraceID
		parentID = remote.SpanID
	}

	span := &Span{
		tracer: t,
		data: SpanData{
			TraceID:      traceID,
			SpanID:       newID(8),
			ParentSpanID: parentID,
			ServiceName:  t.config.ServiceName,
			Name:         name,
			StartTime:    time.Now().UTC(),
			Status:       "ok",
		},
	}
	return context.WithValue(ctx, spanContextKey{}, span), span
}

func (t *Tracer) DroppedSpans() uint64 {
	return t.dropped.Load()
}

func (t *Tracer) Close(ctx context.Context) error {
	t.once.Do(func() {
		t.enqueueMu.Lock()
		t.closed.Store(true)
		close(t.stop)
		t.enqueueMu.Unlock()
	})
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *Tracer) enqueue(span SpanData) {
	t.enqueueMu.RLock()
	defer t.enqueueMu.RUnlock()
	if t.closed.Load() {
		t.dropped.Add(1)
		return
	}
	select {
	case t.queue <- span:
	default:
		t.dropped.Add(1)
		t.config.Logger.Warn("minitrace queue full; dropping span")
	}
}

func (t *Tracer) exportLoop() {
	defer close(t.done)
	ticker := time.NewTicker(t.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]SpanData, 0, t.config.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		t.export(batch)
		batch = batch[:0]
	}

	for {
		select {
		case span := <-t.queue:
			batch = append(batch, span)
			if len(batch) >= t.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-t.stop:
			for {
				select {
				case span := <-t.queue:
					batch = append(batch, span)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (t *Tracer) export(spans []SpanData) {
	body, err := json.Marshal(spanBatch{Spans: spans})
	if err != nil {
		t.config.Logger.Warn("minitrace could not encode spans", "error", err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(t.config.Endpoint, "/")+"/api/v1/spans", bytes.NewReader(body))
	if err != nil {
		t.config.Logger.Warn("minitrace could not create export request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := t.config.HTTPClient.Do(req)
	if err != nil {
		t.config.Logger.Debug("minitrace export failed", "error", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		t.config.Logger.Debug("minitrace export rejected", "status", response.StatusCode)
	}
}
