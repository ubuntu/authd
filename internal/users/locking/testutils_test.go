package userslocking

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

func TestExternalLockCleanupStopsPendingAttempts(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		name := "waiting"
		if delayed {
			name = "not_started"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				result := make(chan error, 1)
				start := make(chan struct{})
				t.Cleanup(func() {
					close(start)
					synctest.Wait()
					select {
					case err := <-result:
						require.ErrorIs(t, err, ErrLockTimeout)
					default:
						t.Fatal("Mock lock attempt did not stop after cleanup")
					}
					require.NoError(t, defaultMock.WriteLock(), "Cleanup must not leave a lock held")
					require.NoError(t, defaultMock.WriteUnlock())
				})

				Z_ForTests_OverrideLockingAsLockedExternally(t, context.Background())
				lock := writeLockImpl
				go func() {
					if delayed {
						<-start
					}
					result <- lock()
				}()
				synctest.Wait()
			})
		})
	}
}
