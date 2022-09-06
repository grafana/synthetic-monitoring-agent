// The feature package provides types and methods for working with
// feature flags.
package feature

import (
	"errors"
	"sort"
	"strings"
)

// TODO: this doesn't seem like the right place for this
const (
	Traceroute = "traceroute"
	AdHoc      = "adhoc"
	K6         = "k6"
)

// ErrInvalidCollection is returned when you try to set a flag in an
// invalid collection.
var ErrInvalidCollection = errors.New("invalid feature collection")

// Collection represents a set of feature flags.
type Collection map[string]struct{}

// NewCollection returns a correctly initialized Collection.
func NewCollection() Collection {
	return make(Collection)
}

// Set adds the value specified by s to the collection.
//
// The input s can be a single feature flag or multiple feature flags
// separated by commas.
func (c Collection) Set(s string) error {
	if c == nil {
		return ErrInvalidCollection
	}

	for _, elem := range strings.Split(s, ",") {
		feature := strings.TrimSpace(elem)
		if feature == "" {
			continue
		}
		c[feature] = struct{}{}
	}

	return nil
}

// String returns an string representation of the collection.
//
// The returned value can be passed to Set to recreate an identical
// collection.
func (c Collection) String() string {
	if c == nil {
		return ""
	}

	values := make([]string, 0, len(c))
	for elem := range c {
		values = append(values, elem)
	}

	sort.Strings(values)

	return strings.Join(values, ",")
}

// IsSet returns true if the provided name is part of the collection,
// false otherwise.
func (c Collection) IsSet(name string) bool {
	if c == nil {
		return false
	}

	_, found := c[name]

	return found
}
