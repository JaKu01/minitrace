package minitrace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const traceparentHeader = "traceparent"

type remoteContext struct {
	TraceID string
	SpanID  string
}

type remoteContextKey struct{}

func remoteContextFromContext(ctx context.Context) (remoteContext, bool) {
	value, ok := ctx.Value(remoteContextKey{}).(remoteContext)
	return value, ok
}

// Middleware traces incoming HTTP requests and understands the W3C traceparent header.
func (t *Tracer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if remote, ok := parseTraceparent(r.Header.Get(traceparentHeader)); ok {
			ctx = context.WithValue(ctx, remoteContextKey{}, remote)
		}
		ctx, span := t.Start(ctx, r.Method+" "+r.URL.Path)
		span.SetAttribute("http.request.method", r.Method).
			SetAttribute("url.path", r.URL.Path)
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			span.SetAttribute("http.response.status_code", writer.status)
			if writer.status >= http.StatusInternalServerError {
				span.SetError(fmt.Errorf("HTTP %d", writer.status))
			}
			span.End()
		}()
		next.ServeHTTP(writer, r.WithContext(ctx))
	})
}

// Transport injects trace context into outgoing HTTP requests.
func (t *Tracer) Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		ctx, span := t.Start(request.Context(), request.Method+" "+request.URL.Host)
		span.SetAttribute("http.request.method", request.Method).
			SetAttribute("server.address", request.URL.Hostname()).
			SetAttribute("url.path", request.URL.Path)

		span.mu.Lock()
		traceparent := fmt.Sprintf("00-%s-%s-01", span.data.TraceID, span.data.SpanID)
		span.mu.Unlock()
		request = request.Clone(ctx)
		request.Header.Set(traceparentHeader, traceparent)

		response, err := base.RoundTrip(request)
		if err != nil {
			span.SetError(err)
			span.End()
			return nil, err
		}
		span.SetAttribute("http.response.status_code", response.StatusCode)
		if response.StatusCode >= http.StatusInternalServerError {
			span.SetError(errors.New(response.Status))
		}
		span.End()
		return response, nil
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func parseTraceparent(value string) (remoteContext, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return remoteContext{}, false
	}
	return remoteContext{TraceID: parts[1], SpanID: parts[2]}, true
}
