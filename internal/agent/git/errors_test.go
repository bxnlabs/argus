package git

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	t.Run("ErrInvalidInput wrapping", func(t *testing.T) {
		err := fmt.Errorf("%w: bad hash %q", ErrInvalidInput, "xyz")
		if !errors.Is(err, ErrInvalidInput) {
			t.Error("expected errors.Is to match ErrInvalidInput")
		}
	})

	t.Run("ErrNotFound wrapping", func(t *testing.T) {
		err := fmt.Errorf("%w: branch %q", ErrNotFound, "nonexistent")
		if !errors.Is(err, ErrNotFound) {
			t.Error("expected errors.Is to match ErrNotFound")
		}
	})

	t.Run("unwrapped errors do not match", func(t *testing.T) {
		err := fmt.Errorf("some other error")
		if errors.Is(err, ErrInvalidInput) {
			t.Error("plain error should not match ErrInvalidInput")
		}
		if errors.Is(err, ErrNotFound) {
			t.Error("plain error should not match ErrNotFound")
		}
	})
}
