package main

// Secret represents a string that shouldn't be logged.
type Secret string

// String returns a redacted version of the secret.
func (s Secret) String() string {
	return "<redacted>"
}

// Set sets the value of the secret.
func (s *Secret) Set(value string) error {
	*s = Secret(value)
	return nil
}

// MarshalText returns a text representation of the redacted version of the
// secret.
//
// This method implements the necessary interface to use Secret in the
// encoding/text and encoding/json packages, which are used by zerolog to
// control the output of the log messages.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}
