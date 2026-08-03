package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	store  *Store
	logger *slog.Logger
	app    fs.FS
}

func New(store *Store, logger *slog.Logger, app fs.FS) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{store: store, logger: logger, app: app}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/spans", s.ingest)
	mux.HandleFunc("GET /api/v1/traces", s.listTraces)
	mux.HandleFunc("GET /api/v1/traces/{traceID}", s.getTrace)
	mux.HandleFunc("GET /api/v1/services", s.services)
	if s.app != nil {
		mux.Handle("/", spaHandler(s.app))
	}
	return s.middleware(mux)
}

func (s *Server) StartRetentionCleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.store.DeleteExpired(contextWithoutCancel{}); err != nil {
				s.logger.Warn("delete expired traces", "error", err)
			}
		case <-stop:
			return
		}
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"retentionHours": s.store.Retention().Hours(),
	})
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var request struct {
		Spans []Span `json:"spans"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(request.Spans) > 1000 {
		writeError(w, http.StatusBadRequest, "a batch may contain at most 1000 spans")
		return
	}
	inserted, err := s.store.InsertSpans(r.Context(), request.Spans)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": inserted})
}

func (s *Server) listTraces(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	traces, err := s.store.ListTraces(
		r.Context(),
		r.URL.Query().Get("service"),
		r.URL.Query().Get("q"),
		r.URL.Query().Get("status"),
		limit,
	)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, traces)
}

func (s *Server) getTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := s.store.GetTrace(
		r.Context(),
		r.PathValue("traceID"),
		strings.TrimSpace(r.URL.Query().Get("service")),
	)
	if errors.Is(err, ErrTraceNotFound) {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	services, err := s.store.Services(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, traceparent")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func spaHandler(app fs.FS) http.Handler {
	files := http.FileServer(http.FS(app))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(app, path); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

// contextWithoutCancel is sufficient for periodic database housekeeping.
type contextWithoutCancel struct{}

func (contextWithoutCancel) Deadline() (time.Time, bool) { return time.Time{}, false }
func (contextWithoutCancel) Done() <-chan struct{}       { return nil }
func (contextWithoutCancel) Err() error                  { return nil }
func (contextWithoutCancel) Value(any) any               { return nil }
