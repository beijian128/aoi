package aoi

type Set[T comparable] map[T]struct{}

func NewSet[T comparable](values ...T) Set[T] {
	s := make(Set[T])
	for _, v := range values {
		s.Add(v)
	}
	return s
}

func (s Set[T]) Add(value T) {
	s[value] = struct{}{}
}
func (s Set[T]) Remove(value T) {
	delete(s, value)
}
func (s Set[T]) Contains(value T) bool {
	_, ok := s[value]
	return ok
}
func (s Set[T]) Empty() bool {
	return len(s) == 0
}
func (s Set[T]) Size() int {
	return len(s)
}
func (s Set[T]) Clear() {
	for k := range s {
		delete(s, k)
	}
}
func (s Set[T]) ForEach(f func(value T)) {
	for k := range s {
		f(k)
	}
}
