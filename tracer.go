package minitrace

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var Spans []*Span
var serverStarted = atomic.Bool{}

func ensureServerStarted() {
	if serverStarted.CompareAndSwap(false, true) {
		go serveHttp()
	}
}

func Start(ctx context.Context) (*Span, context.Context) {
	ensureServerStarted()

	parentID := "root"

	parent, ok := ctx.Value(activeSpanKey).(*Span)
	if ok {
		parentID = parent.ID
	}

	span := &Span{
		ID:        uuid.NewString(),
		ParentID:  parentID,
		Name:      getCaller(),
		StartTime: time.Now(),
	}

	Spans = append(Spans, span)

	ctx = context.WithValue(ctx, activeSpanKey, span)
	return span, ctx
}

func (s *Span) End() {
	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
}
