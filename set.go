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

func (s Set[T]) ForEach(f func(value T) bool) {
	for k := range s {
		if f(k) {
			break
		}
	}
}

// Union 并集
func (s Set[T]) Union(other Set[T]) Set[T] {
	result := NewSet[T]()
	for v := range s {
		result.Add(v)
	}
	for v := range other {
		result.Add(v)
	}
	return result
}

// Intersection 交集
func (s Set[T]) Intersection(other Set[T]) Set[T] {
	result := NewSet[T]()
	// 遍历较小的集合以获得更好的性能
	if len(s) < len(other) {
		for v := range s {
			if other.Contains(v) {
				result.Add(v)
			}
		}
	} else {
		for v := range other {
			if s.Contains(v) {
				result.Add(v)
			}
		}
	}
	return result
}

// Difference 差集 （返回仅S中有的元素）
func (s Set[T]) Difference(other Set[T]) Set[T] {
	result := NewSet[T]()
	for v := range s {
		if !other.Contains(v) {
			result.Add(v)
		}
	}
	return result
}
