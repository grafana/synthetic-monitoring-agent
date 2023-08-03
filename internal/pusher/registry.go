package pusher

import (
	"fmt"
	"sort"
	"sync"
)

// registry to register and obtain instances of the named type.
//
// This type is not exported to prevent creating instances without calling NewRegistry.
type registry[T any] struct {
	mu      sync.Mutex
	entries map[string]T
}

func NewRegistry[T any]() *registry[T] {
	var r registry[T]
	r.entries = make(map[string]T)
	return &r
}

func (r *registry[T]) Register(name string, handler T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, found := r.entries[name]; found {
		return AlreadyRegisteredError(name)
	}
	r.entries[name] = handler
	return nil
}

func (r *registry[T]) MustRegister(name string, handler T) {
	if err := r.Register(name, handler); err != nil {
		panic(err)
	}
}

func (r *registry[T]) Lookup(name string) (T, error) {
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
