package blade_test

import (
	"testing"

	"github.com/mertenvg/blade/pkg/blade"
)

func TestDone_DoesNotPanic(t *testing.T) {
	// Done is a deprecated no-op. Verify it remains callable without panic
	// for backward compatibility with services that still call it.
	blade.Done()
}
