package stackset

type (
	StackSet[T comparable] struct {
		set   map[T]struct{}
		stack []T
	}
)

func New[T comparable]() *StackSet[T] {
	return &StackSet[T]{
		set:   map[T]struct{}{},
		stack: nil,
	}
}

func (s *StackSet[T]) Push(v T) bool {
	if _, ok := s.set[v]; ok {
		return false
	}
	s.set[v] = struct{}{}
	s.stack = append(s.stack, v)
	return true
}

func (s *StackSet[T]) Pop() (T, bool) {
	var v T
	l := len(s.stack)
	if l == 0 {
		return v, false
	}
	v = s.stack[l-1]
	s.stack = s.stack[:l-1]
	delete(s.set, v)
	return v, true
}

// Slice returns the stack, however the stack must not be modified
func (s *StackSet[T]) Slice() []T {
	ret := make([]T, len(s.stack))
	copy(ret, s.stack)
	return ret
}
