package syncmap

import "sync"

type Map[K comparable, V any] struct {
	m sync.Map
}

func (m *Map[K, V]) Delete(key K) { m.m.Delete(key) }

func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.m.Load(key)
	if !ok {
		return value, ok
	}
	// Type assertion with safety check
	if typedValue, typeOk := v.(V); typeOk {
		return typedValue, ok
	}
	// If type assertion fails, return zero value and false
	return value, false
}

func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, loaded := m.m.LoadAndDelete(key)
	if !loaded {
		return value, loaded
	}
	// Type assertion with safety check
	if typedValue, typeOk := v.(V); typeOk {
		return typedValue, loaded
	}
	// If type assertion fails, return zero value and false for loaded
	return value, false
}

func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	a, loaded := m.m.LoadOrStore(key, value)
	// Type assertion with safety check
	if typedValue, typeOk := a.(V); typeOk {
		return typedValue, loaded
	}
	// If type assertion fails, return the provided value and loaded status
	return value, loaded
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value any) bool {
		// Type assertions with safety checks
		typedKey, keyOk := key.(K)
		typedValue, valueOk := value.(V)
		if !keyOk || !valueOk {
			return true // Continue iteration if type assertion fails
		}
		return f(typedKey, typedValue)
	})
}

func (m *Map[K, V]) Store(key K, value V) { m.m.Store(key, value) }
