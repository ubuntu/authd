package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/dbusmodule"
)

var (
	pamFlags = flag.Int64("flags", 0, "pam flags")
)

func init() {
	// We need to stay on the main thread all the time here, to make sure we're
	// calling the dbus services from the process and so that the module PID
	// check won't fail.
	runtime.LockOSThread()
}

func mainFunc() error {
	module := &pamModule{}

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		return errors.New("not enough arguments")
	}

	serverAddress := os.Getenv("AUTHD_PAM_SERVER_ADDRESS")
	if serverAddress == "" {
		return fmt.Errorf("%w: no connection provided", pam.ErrSystem)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mTx, closeFunc, err := dbusmodule.NewTransaction(ctx, serverAddress)
	if err != nil {
		return fmt.Errorf("%w: can't connect to server: %w", pam.ErrSystem, err)
	}
	defer closeFunc()

	actionDone := make(chan struct{})
	defer close(actionDone)

	go func() {
		select {
		case <-actionDone:
		case <-mTx.Context().Done():
			log.Warningf(context.Background(), "D-Bus Connection closed: %v",
				mTx.Context().Err())
			os.Exit(255)
		}
	}()

	action, args := args[0], args[1:]

	flags := pam.Flags(0)
	if pamFlags != nil {
		flags = pam.Flags(*pamFlags)
	}

	log.Debugf(context.Background(), "Starting action %q (%v)", action, flags)

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
