package trace

import (
	"time"
)

type spanKey struct{}

var activeSpanKey = spanKey{}

type Span struct {
	ID        string
	ParentID  string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}
