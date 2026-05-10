package trace

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/JaKu01/minitrace/internal/api"
)

func buildTree(spans []*Span, lastFetchedTimestamp time.Time) api.SpanApiResponse {

	var rootChildren []Span
	useTimestampFilter := lastFetchedTimestamp.IsZero()
	for _, span := range spans {
		if useTimestampFilter && span.StartTime.Before(lastFetchedTimestamp) {
			continue
		}

		if span.ParentID == "root" {
			rootChildren = append(rootChildren, *span)
		}
	}

	childrenByParentID := buildChildrenByParentIDMap(spans)

	var rootChildDtos []api.SpanDTO
	for _, rootChild := range rootChildren {
		rootChildDtos = append(rootChildDtos, getChildrenRecursive(childrenByParentID, rootChild))
	}

	return api.SpanApiResponse{
		Children: rootChildDtos,
	}
}

func getChildrenRecursive(childrenByParentID map[string][]*Span, span Span) api.SpanDTO {
	children := childrenByParentID[span.ID]

	childDtos := make([]api.SpanDTO, 0)
	for _, child := range children {
		childDtos = append(childDtos, getChildrenRecursive(childrenByParentID, *child))
	}

	return api.SpanDTO{
		Id:        span.ID,
		Name:      span.Name,
		StartTime: span.StartTime,
		EndTime:   span.EndTime,
		Duration:  int64(span.Duration),
		Children:  childDtos,
	}
}

func buildChildrenByParentIDMap(spans []*Span) map[string][]*Span {
	childrenByParentID := make(map[string][]*Span)

	for _, span := range spans {
		childrenByParentID[span.ParentID] = append(childrenByParentID[span.ParentID], span)
	}

	return childrenByParentID
}

func serveHttp() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/traces", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(buildTree(Spans, time.Time{}))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("Failed to start http server", "err", err.Error())
	}
}
