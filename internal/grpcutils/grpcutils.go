// Package grpcutils provides utility functions for GRPC operations.
package grpcutils

import (
	"context"
	"fmt"
	"time"

	"github.com/ubuntu/authd/internal/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// WaitForConnection synchronously waits for a [grpc.ClientConn] connection to be established.
func WaitForConnection(ctx context.Context, conn *grpc.ClientConn, timeout time.Duration) (err error) {
	// Block for connection to be started.
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Debugf(ctx, "Connecting to %s", conn.Target())
	conn.Connect()

	for {
		switch state := conn.GetState(); state {
		// In case of connectivity.TransientFailure we can't fail early since we may
		// still connect in time for the timeout, so we have to be conservative here.
		case connectivity.Ready:
			return nil
		}

		conn.WaitForStateChange(waitCtx, conn.GetState())
		if err := waitCtx.Err(); err != nil {
			return fmt.Errorf("could not connect to %v: %w", conn.Target(), err)
		}
	}
}
