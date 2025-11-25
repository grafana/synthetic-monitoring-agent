package error_types

// BasicError is an error for which we have no additional information.
type BasicError string

func (e BasicError) Error() string { return string(e) }

var _ error = BasicError("")

// ValidationError is an error for which we have no additional information.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

var _ error = ValidationError("")

// TransientError is an error that can be recovered.
type TransientError BasicError

func (e TransientError) Error() string { return string(e) }

var _ error = TransientError("")

// FatalError is an error that causes the program to terminate.
type FatalError BasicError

func (e FatalError) Error() string { return string(e) }

var _ error = FatalError("")
