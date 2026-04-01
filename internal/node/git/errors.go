package git

import "errors"

// ErrInvalidInput is returned when the caller provides syntactically invalid
// input (bad commit hash format, invalid branch name, etc.).
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound is returned when a referenced git object does not exist
// (nonexistent branch, unknown revision, etc.).
var ErrNotFound = errors.New("not found")

// ErrFileTooLarge is returned when a file exceeds the size limit for
// line-based reads (2 MB).
var ErrFileTooLarge = errors.New("file too large")

// ErrBinaryFile is returned when a binary file is detected during
// line-based reads.
var ErrBinaryFile = errors.New("binary file")
