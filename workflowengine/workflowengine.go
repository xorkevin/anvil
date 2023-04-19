package workflowengine

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"xorkevin.dev/kerrors"
)

var (
	// ErrNotSupported is returned when the kind is not supported
	ErrNotSupported errNotSupported
	// ErrInvalidArgs is returned when calling an engine native function with invalid args
	ErrInvalidArgs errInvalidArgs
)

type (
	errNotSupported struct{}
	errInvalidArgs  struct{}
)

func (e errNotSupported) Error() string {
	return "Engine kind not supported"
}

func (e errInvalidArgs) Error() string {
	return "Invalid args"
}

type (
	// WorkflowEngine is a workflow engine
	WorkflowEngine interface {
		Exec(ctx context.Context, events *EventHistory, name string, fn string, args map[string]any, stdout io.Writer) (any, error)
	}

	// Builder builds a [WorkflowEngine]
	Builder interface {
		Build(fsys fs.FS) (WorkflowEngine, error)
	}

	// BuilderFunc implements Builder for a function
	BuilderFunc func(fsys fs.FS) (WorkflowEngine, error)

	// Map is a map from kinds to [Builder]
	Map map[string]Builder
)

func (f BuilderFunc) Build(fsys fs.FS) (WorkflowEngine, error) {
	return f(fsys)
}

// Build builds a [WorkflowEngine] for a known kind
func (m Map) Build(kind string, fsys fs.FS) (WorkflowEngine, error) {
	a, ok := m[kind]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotSupported, fmt.Sprintf("Engine kind not supported: %s", kind))
	}
	eng, err := a.Build(fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build workflow engine")
	}
	return eng, nil
}

type (
	// EventHistory is an append only log of workflow events
	EventHistory struct {
		idx     int
		history []Event
	}

	// Event is a workflow event
	Event struct {
		Key   any
		Value any
	}
)

// NewEventHistory creates a new [EventHistory]
func NewEventHistory() *EventHistory {
	return &EventHistory{
		idx:     0,
		history: nil,
	}
}

func (h *EventHistory) Next() (Event, bool) {
	if h.idx >= len(h.history) {
		return Event{}, false
	}
	e := h.history[h.idx]
	h.idx++
	return e, true
}

func (h *EventHistory) Push(key any, value any) {
	h.history = append(h.history, Event{
		Key:   key,
		Value: value,
	})
	h.idx = len(h.history)
}

func (h *EventHistory) Start() {
	h.idx = 0
}

func (h *EventHistory) Index() int {
	return h.idx
}
