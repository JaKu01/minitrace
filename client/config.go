package minitrace

import (
	"log/slog"
	"net/http"
	"time"
)

const (
	defaultEndpoint      = "http://localhost:8080"
	defaultFlushInterval = 500 * time.Millisecond
	defaultBatchSize     = 100
	defaultQueueSize     = 2048
)

// Config controls how a Tracer exports spans.
type Config struct {
	ServiceName   string
	Endpoint      string
	FlushInterval time.Duration
	BatchSize     int
	QueueSize     int
	HTTPClient    *http.Client
	Logger        *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaultFlushInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaultQueueSize
	}
	if c.HTTPClient == nil {
		// Pin the current transport so a later application-level wrapper around
		// http.DefaultTransport cannot trace the exporter's own requests.
		c.HTTPClient = &http.Client{
			Timeout:   3 * time.Second,
			Transport: http.DefaultTransport,
		}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}
