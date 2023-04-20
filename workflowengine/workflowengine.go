package workflowengine

import (
	"context"
	"encoding/json"
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

	// ActivityReturnEventKey wraps an activity args event key
	ActivityReturnEventKey struct {
		Key any
	}

	Activity interface {
		Key() any
		Serialize() (any, error)
		Exec(ctx context.Context) (any, error)
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

func (h *EventHistory) ExecActivity(ctx context.Context, activity Activity) (any, error) {
	key := activity.Key()

	serial, err := activity.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Failed to serialize activity %v args at event history index %d: %w", key, h.idx, err)
	}
	if e, ok := h.Next(); ok {
		if key != e.Key {
			return nil, fmt.Errorf("Args event key mismatch for activity function at event history index %d: want %v, received %v", h.idx, e.Key, key)
		}
		if !deepEqualAny(serial, e.Value) {
			return nil, fmt.Errorf("Args event value mismatch for activity function %v at event history index %d", key, h.idx)
		}
	} else {
		h.Push(key, serial)
	}

	retKey := ActivityReturnEventKey{Key: key}
	if e, ok := h.Next(); ok {
		if e.Key != retKey {
			return nil, fmt.Errorf("Return event key mismatch for activity function at event history index %d: want %s, received %s", h.idx, e.Key, retKey)
		}
		return e.Value, nil
	}

	value, err := activity.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error executing activity %v at event log index %d: %w", key, h.idx, err)
	}

	h.Push(retKey, value)

	return value, nil
}

func (k ActivityReturnEventKey) String() string {
	return fmt.Sprintf("return:%v", k.Key)
}

func equalScalar[T comparable](a T, b any) bool {
	bx, ok := b.(T)
	if !ok {
		return false
	}
	return a == bx
}

func deepEqualAny(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch ax := a.(type) {
	case bool:
		return equalScalar(ax, b)
	case int:
		return equalScalar(ax, b)
	case int8:
		return equalScalar(ax, b)
	case int16:
		return equalScalar(ax, b)
	case int32:
		return equalScalar(ax, b)
	case int64:
		return equalScalar(ax, b)
	case uint:
		return equalScalar(ax, b)
	case uint8:
		return equalScalar(ax, b)
	case uint16:
		return equalScalar(ax, b)
	case uint32:
		return equalScalar(ax, b)
	case uint64:
		return equalScalar(ax, b)
	case uintptr:
		return equalScalar(ax, b)
	case float32:
		return equalScalar(ax, b)
	case float64:
		return equalScalar(ax, b)
	case json.Number:
		return equalScalar(ax, b)
	case string:
		return equalScalar(ax, b)
	case map[string]any:
		{
			bx, ok := b.(map[string]any)
			if !ok {
				return false
			}
			if len(ax) != len(bx) {
				return false
			}
			for k, v := range ax {
				if !deepEqualAny(v, bx[k]) {
					return false
				}
			}
			return true
		}
	case []any:
		{
			bx, ok := b.([]any)
			if !ok {
				return false
			}
			if len(ax) != len(bx) {
				return false
			}
			for n, i := range ax {
				if !deepEqualAny(i, bx[n]) {
					return false
				}
			}
			return true
		}
	default:
		return false
	}
}
