// Package pb is empty and exists only to express build dependencies for
// protocol buffers code generation and to provide a nice clean way to
// obtain the location of the subpackages using "go list".
package pb

// This blank import below is needed because gogoproto needs
// annotations.proto, which is part of the "api" package.
import _ "github.com/gogo/googleapis/google/api"
