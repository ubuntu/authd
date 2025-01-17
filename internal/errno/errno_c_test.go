package errno_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/errno"
)

func TestNoError(t *testing.T) {
	t.Parallel()

	errno.Lock()
	t.Cleanup(errno.Unlock)

	require.NoError(t, errno.Get())
}

func TestGetWithoutLocking(t *testing.T) {
	// This test can't be parallel, since other tests may locking meanwhile.

	require.PanicsWithValue(t, "Using errno without locking!", func() { _ = errno.Get() })
}

func TestSetWithoutLocking(t *testing.T) {
	// This test can't be parallel, since other tests may locking meanwhile.

	require.PanicsWithValue(t, "Using errno without locking!", func() { errno.Set(nil) })
}

func TestSetInvalidError(t *testing.T) {
	t.Parallel()

	errno.Lock()
	t.Cleanup(errno.Unlock)

	require.PanicsWithValue(t, "Not a valid errno value", func() { errno.Set(errors.New("invalid")) })
}

func TestErrorValues(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		err error
	}{
		"No error":                  {},
		"No such file or directory": {err: errno.ErrNoEnt},
		"No such process":           {err: errno.ErrSrch},
		"Bad file descriptor":       {err: errno.ErrBadf},
		"Operation not permitted":   {err: errno.ErrPerm},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			errno.Lock()
			t.Cleanup(errno.Unlock)

			errno.Set(tc.err)
			t.Logf("Checking for error %v", tc.err)
			require.ErrorIs(t, errno.Get(), tc.err, "Errno is not matching")
		})
	}
}
