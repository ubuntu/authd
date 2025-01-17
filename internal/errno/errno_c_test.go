package errno

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoError(t *testing.T) {
	t.Parallel()

	Lock()
	t.Cleanup(Unlock)

	require.NoError(t, Get())
}

func TestGetWithoutLocking(t *testing.T) {
	// This test can't be parallel, since other tests may locking meanwhile.

	require.PanicsWithValue(t, "Using errno without locking!", func() { _ = Get() })
}

func TestSetWithoutLocking(t *testing.T) {
	// This test can't be parallel, since other tests may locking meanwhile.

	require.PanicsWithValue(t, "Using errno without locking!", func() { set(nil) })
}

func TestSetInvalidError(t *testing.T) {
	t.Parallel()

	Lock()
	t.Cleanup(Unlock)

	require.PanicsWithValue(t, "Not a valid errno value", func() { set(errors.New("invalid")) })
}

func TestErrorValues(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		err error
	}{
		"No error":                  {},
		"No such file or directory": {err: ErrNoEnt},
		"No such process":           {err: ErrSrch},
		"Bad file descriptor":       {err: ErrBadf},
		"Operation not permitted":   {err: ErrPerm},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			Lock()
			t.Cleanup(Unlock)

			set(tc.err)
			t.Logf("Checking for error %v", tc.err)
			require.ErrorIs(t, Get(), tc.err, "Errno is not matching")
		})
	}
}
