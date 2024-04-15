// Package permissions handles peer user detection and permissions.
package permissions

import (
	"context"
	"errors"
	"fmt"

	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc/peer"
)

var permErrorFmt = "this action is only allowed for root users. Current user is %d"

// Manager is an abstraction of permission process.
type Manager struct {
	rootUID uint32
}

type options struct {
	rootUID uint32
}

var defaultOptions = options{
	rootUID: 0,
}

// Option represents an optional function to override Manager default values.
type Option func(*options)

// New returns a new Manager.
func New(args ...Option) Manager {
	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	//nolint:gosimple // S1016 Those structs are not the same conceptually.
	return Manager{
		rootUID: opts.rootUID,
	}
}

// IsRequestFromRoot returns nil if the request was performed by a root user.
// The pid and uid are extracted from peerCredsInfo in the gRPC context.
func (m Manager) IsRequestFromRoot(ctx context.Context) (err error) {
	log.Debug(ctx, "Check if this grpc call is requested by root")
	defer decorate.OnError(&err, "permission denied")

	p, ok := peer.FromContext(ctx)
	if !ok {
		return errors.New("context request doesn't have grpc peer informations")
	}
	pci, ok := p.AuthInfo.(peerCredsInfo)
	if !ok {
		return errors.New("context request doesn't have valid grpc peer credential informations")
	}

	if pci.uid != m.rootUID {
		return fmt.Errorf(permErrorFmt, pci.uid)
	}

	return nil
}
