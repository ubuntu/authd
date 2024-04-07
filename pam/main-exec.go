//go:build !pam_binary_cli

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/dbusmodule"
	"github.com/ubuntu/authd/pam/internal/gdm"
)

var (
	pamFlags      = flag.Int64("flags", 0, "pam flags")
	serverAddress = flag.String("server-address", "", "the dbus connection to use to communicate with module")
	enableGdm     = flag.Bool("enable-gdm", false, "toggle to enable GDM protocol")
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

	if serverAddress == nil {
		return fmt.Errorf("%w: no connection provided", pam.ErrSystem)
	}

	mTx, closeFunc, err := dbusmodule.NewTransaction(context.TODO(), *serverAddress)
	if err != nil {
		return fmt.Errorf("%w: can't connect to server: %w", pam.ErrSystem, err)
	}
	defer closeFunc()

	action, args := args[0], args[1:]

	// We parse again the remaining arguments again
	err = flag.CommandLine.Parse(args)
	if err != nil {
		return fmt.Errorf("%w: can't parse arguments: %w", pam.ErrSystem, err)
	}
	args = flag.CommandLine.Args()

	flags := pam.Flags(0)
	if pamFlags != nil {
		flags = pam.Flags(*pamFlags)
	}

	if enableGdm != nil && *enableGdm {
		gdm.AdvertisePamExtensions([]string{gdm.PamExtensionCustomJSON})
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
