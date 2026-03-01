package git

import "errors"

// ErrInvalidInput is returned when the caller provides syntactically invalid
// input (bad commit hash format, invalid branch name, etc.).
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound is returned when a referenced git object does not exist
// (nonexistent branch, unknown revision, etc.).
var ErrNotFound = errors.New("not found")
