package autofix

import (
	"context"
	"testing"
)

func TestShapes(t *testing.T) {
	t.Parallel()
	call := func() error { return nil }
	query := func(context.Context, int) error { return nil }
	withContext := func(context.Context) error { return nil }
	withString := func(string) error { return nil }
	checks := []error{
		ElseIf(t.Context(), 1, false, query),
		OuterError(call),
		Timeout(t.Context(), withContext),
		Imports([]byte("value"), withString),
	}
	for _, err := range checks {
		if err != nil {
			t.Fatal(err)
		}
	}
}
