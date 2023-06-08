package pusher

import (
	"fmt"
	"sort"
	"sync"
)

type Registry[T any] struct {
	mu      sync.Mutex
	entries map[string]T
}

func (r *Registry[T]) Register(name string, handler T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, found := r.entries[name]; found {
		return AlreadyRegisteredError(name)
	}
	if r.entries == nil {
		r.entries = make(map[string]T)
	}
	r.entries[name] = handler
	return nil
}

func (r *Registry[T]) MustRegister(name string, handler T) {
	if err := r.Register(name, handler); err != nil {
		panic(err)
	}
}

func (r *Registry[T]) Lookup(name string) (T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, found := r.entries[name]
	if !found {
		var (
			zero T
			keys []string
		)
		for key := range r.entries {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return zero, NotFoundError{
			Requested: name,
			Accepted:  keys,
		}
	}
	return entry, nil
}

type NotFoundError struct {
	Requested string
	Accepted  []string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("`%s` not found. Must be one of %v", e.Requested, e.Accepted)
}

type AlreadyRegisteredError string

func (e AlreadyRegisteredError) Error() string {
	return fmt.Sprintf("entry `%s` already registered", string(e))
}
