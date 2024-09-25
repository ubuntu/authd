// Package daemon handles the GRPC daemon with systemd support.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/coreos/go-systemd/activation"
	"github.com/coreos/go-systemd/daemon"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc"
)

// Daemon is a grpc daemon with systemd support.
type Daemon struct {
	grpcServer *grpc.Server
	lis        net.Listener

	systemdSdNotifier systemdSdNotifier
}

type options struct {
	socketPath string

	// private member that we export for tests.
	systemdActivationListener func() ([]net.Listener, error)
	systemdSdNotifier         func(unsetEnvironment bool, state string) (bool, error)
}

type systemdSdNotifier func(unsetEnvironment bool, state string) (bool, error)

// Option is the function signature used to tweak the daemon creation.
type Option func(*options)

// WithSocketPath uses a manual socket path instead of socket activation.
func WithSocketPath(p string) func(o *options) {
	return func(o *options) {
		o.socketPath = p
	}
}

// GRPCServiceRegisterer is a function that the daemon will call everytime we want to build a new GRPC object.
type GRPCServiceRegisterer func(context.Context) *grpc.Server

// New returns an new, initialized daemon server, which handles systemd activation.
// If systemd activation is used, it will override any socket passed here.
func New(ctx context.Context, registerGRPCService GRPCServiceRegisterer, args ...Option) (d *Daemon, err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "can't create daemon") //)

	slog.Debug("Building new daemon")

	// Set default options.
	opts := options{
		socketPath: "",

		systemdActivationListener: activation.Listeners,
		systemdSdNotifier:         daemon.SdNotify,
	}
	// Apply given args.
	for _, f := range args {
		f(&opts)
	}

	// systemd socket activation or local creation
	var lis net.Listener

	if opts.socketPath != "" {
		slog.Debug(fmt.Sprintf("Listening on %s", opts.socketPath))

		// manual socket
		// TODO: if socket exists, remove
		lis, err = net.Listen("unix", opts.socketPath)
		if err != nil {
			return nil, err
		}

		//nolint:gosec // We want everyone to be able to write to our socket and we will filter permissions
		if err = os.Chmod(opts.socketPath, 0666); err != nil {
			return nil, fmt.Errorf("could not change socket permission: %v", err)
		}
	} else {
		slog.Debug("Use socket activation")

		// systemd activation
		listeners, err := opts.systemdActivationListener()
		if err != nil {
			return d, err
		}

		if len(listeners) != 1 {
			return nil, fmt.Errorf( /*i18n.G(*/ "unexpected number of systemd socket activation (%d != 1)" /*)*/, len(listeners))
		}
		lis = listeners[0]
	}

	// Ensure selected socket exists.
	if _, err := os.Stat(lis.Addr().String()); err != nil {
		return nil, fmt.Errorf("%s can’t be acccessed: %v", lis.Addr().String(), err)
	}

	return &Daemon{
		grpcServer: registerGRPCService(ctx),
		lis:        lis,

		systemdSdNotifier: opts.systemdSdNotifier,
	}, nil
}

// Serve listens on a tcp socket and starts serving GRPC requests on it.
func (d *Daemon) Serve(ctx context.Context) (err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "error while serving") //)

	slog.Debug(fmt.Sprintf("Starting to serve requests on %s", d.lis.Addr()))

	// Signal to systemd that we are ready.
	if sent, err := d.systemdSdNotifier(false, "READY=1"); err != nil {
		return fmt.Errorf( /*i18n.G(*/ "couldn't send ready notification to systemd: %v" /*)*/, err)
	} else if sent {
		slog.Debug("Ready state sent to systemd")
	}

	slog.Info(fmt.Sprintf("Serving GRPC requests on %v", d.lis.Addr()))
	if err := d.grpcServer.Serve(d.lis); err != nil {
		return fmt.Errorf("grpc error: %v", err)
	}
	return nil
}

// Quit gracefully quits listening loop and stops the grpc server.
// It can drops any existing connexion is force is true.
func (d Daemon) Quit(ctx context.Context, force bool) {
	slog.Info("Stopping daemon requested.")
	if force {
		d.grpcServer.Stop()
		return
	}

	slog.Info("Wait for active requests to close.")
	d.grpcServer.GracefulStop()
	slog.Debug("All connections have now ended.")
}
