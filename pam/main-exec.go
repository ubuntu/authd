package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/dbusmodule"
	"golang.org/x/sys/unix"
)

var (
	pamFlags      = flag.Int64("flags", 0, "pam flags")
	serverAddress = flag.String("server-address", "", "the dbus connection to use to communicate with module")
	timeout       = flag.Int64("timeout", 120, "timeout for the server connection (in seconds)")
)

func init() {
	// We need to stay on the main thread all the time here, to make sure we're
	// calling the dbus services from the process and so that the module PID
	// check won't fail.
	runtime.LockOSThread()

	// Ask the kernel to send SIGTERM if the parent dies
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0, 0, 0); err != nil {
		log.Errorf(context.Background(), "failed to set PDEATHSIG: %v", err)
		os.Exit(1)
	}

	// Check if parent is still alive
	ppid := unix.Getppid()
	if ppid == 1 {
		log.Error(context.Background(), "parent is already gone; exiting")
		os.Exit(1)
	}
}

func mainFunc() error {
	module := &pamModule{}

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		return errors.New("not enough arguments")
	}

	serverAddressEnv := os.Getenv("AUTHD_PAM_SERVER_ADDRESS")
	if serverAddressEnv != "" {
		*serverAddress = serverAddressEnv
	}

	if serverAddress == nil {
		return fmt.Errorf("%w: no connection provided", pam.ErrSystem)
	}

	ctx, cancel := context.WithTimeout(context.TODO(), time.Duration(*timeout)*time.Second)
	defer cancel()
	mTx, closeFunc, err := dbusmodule.NewTransaction(ctx, *serverAddress)
	if err != nil {
		return fmt.Errorf("%w: can't connect to server: %w", pam.ErrSystem, err)
	}
	defer closeFunc()

	action, args := args[0], args[1:]

	flags := pam.Flags(0)
	if pamFlags != nil {
		flags = pam.Flags(*pamFlags)
	}

	switch action {
	case "authenticate":
		return module.Authenticate(mTx, flags, args)
	case "acct_mgmt":
		return module.AcctMgmt(mTx, flags, args)
	case "open_session":
		return module.OpenSession(mTx, flags, args)
	case "close_session":
		return module.CloseSession(mTx, flags, args)
	case "chauthtok":
		return module.ChangeAuthTok(mTx, flags, args)
	case "setcred":
		return module.SetCred(mTx, flags, args)
	default:
		return fmt.Errorf("unknown action %s: %v", action, pam.ErrSystem)
	}
}

func main() {
	err := mainFunc()
	if err == nil {
		os.Exit(0)
	}
	var pamError pam.Error
	if !errors.As(err, &pamError) {
		log.Error(context.TODO(), err)
		os.Exit(255)
	}
	os.Exit(int(pamError))
}
