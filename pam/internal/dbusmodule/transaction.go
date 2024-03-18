// Package dbusmodule includes the tool for DBus PAM module interactions.
package dbusmodule

import (
	"context"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

// Transaction is a [pam.Transaction] with dbus support.
type Transaction struct {
	pam.ModuleTransaction
	obj dbus.BusObject
}

type options struct {
	isSharedConnection bool
}

// TransactionOptions is the function signature used to tweak the transaction.
type TransactionOptions func(*options)

// WithSharedConnection indicates that we're using a shared dbus connection.
func WithSharedConnection(isShared bool) func(o *options) {
	return func(o *options) {
		o.isSharedConnection = true
	}
}

const ifaceName = "com.ubuntu.authd.pam"
const objectPath = "/com/ubuntu/authd/pam"

// NewTransaction creates a new [dbusmodule.Transaction] with the provided connection.
// A [pam.ModuleTransaction] implementation is returned together with a cleanup function that
// should be called to release the connection.
func NewTransaction(ctx context.Context, address string, o ...TransactionOptions) (tx pam.ModuleTransaction, cleanup func(), err error) {
	opts := options{}
	for _, f := range o {
		f(&opts)
	}

	log.Debugf(context.TODO(), "Connecting to %s", address)
	conn, err := dbus.Dial(address, dbus.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { conn.Close() }
	if err = conn.Auth(nil); err != nil {
		cleanup()
		return nil, nil, err
	}
	if opts.isSharedConnection {
		if err = conn.Hello(); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	obj := conn.Object(ifaceName, objectPath)
	return &Transaction{
		ModuleTransaction: &pam_test.ModuleTransactionDummy{},
		obj:               obj,
	}, cleanup, nil
}

// SetData allows to save any value in the module data that is preserved
// during the whole time the module is loaded.
func (tx *Transaction) SetData(key string, data any) error {
	if data == nil {
		return dbusUnsetter(tx.obj, "UnsetData", key)
	}
	return dbusSetter(tx.obj, "SetData", key, dbus.MakeVariant(data))
}

// GetData allows to get any value from the module data saved using SetData
// that is preserved across the whole time the module is loaded.
func (tx *Transaction) GetData(key string) (any, error) {
	return dbusGetter[any](tx.obj, "GetData", key)
}

// SetItem sets a PAM item.
func (tx *Transaction) SetItem(item pam.Item, value string) error {
	return dbusSetter(tx.obj, "SetItem", item, value)
}

// GetItem retrieves a PAM item.
func (tx *Transaction) GetItem(item pam.Item) (string, error) {
	return dbusGetter[string](tx.obj, "GetItem", item)
}

// PutEnv adds or changes the value of PAM environment variables.
//
// NAME=value will set a variable to a value.
// NAME= will set a variable to an empty value.
// NAME (without an "=") will delete a variable.
func (tx *Transaction) PutEnv(nameVal string) error {
	if !strings.Contains(nameVal, "=") {
		return dbusUnsetter(tx.obj, "UnsetEnv", nameVal)
	}
	envPair := strings.SplitN(nameVal, "=", 2)
	return dbusSetter(tx.obj, "SetEnv", envPair[0], envPair[1])
}

// GetEnv is used to retrieve a PAM environment variable.
func (tx *Transaction) GetEnv(name string) string {
	env, err := dbusGetter[string](tx.obj, "GetEnv", name)
	if err != nil {
		return ""
	}
	return env
}

// GetEnvList returns a copy of the PAM environment as a map.
func (tx *Transaction) GetEnvList() (map[string]string, error) {
	var r int
	var envMap map[string]string
	method := fmt.Sprintf("%s.GetEnvList", ifaceName)
	err := tx.obj.Call(method, dbus.FlagNoAutoStart).Store(&r, &envMap)
	if err != nil {
		log.Debugf(context.TODO(), "failed to call %s: %v", method, err)
		return nil, fmt.Errorf("%w: %w", pam.ErrSystem, err)
	}
	if r != 0 {
		log.Debugf(context.TODO(), "failed to call %s: %s", method, pam.Error(r))
		return nil, pam.Error(r)
	}
	return envMap, nil
}

// InvokeHandler is called by the C code to invoke the proper handler.
func (tx *Transaction) InvokeHandler(handler pam.ModuleHandlerFunc,
	flags pam.Flags, args []string) error {
	return pam.ErrAbort
}

func dbusSetter[V any, K any](obj dbus.BusObject, method string, key K, value V) error {
	var r int
	method = fmt.Sprintf("%s.%s", ifaceName, method)
	err := obj.Call(method, dbus.FlagNoAutoStart, key, value).Store(&r)
	if err != nil {
		log.Debugf(context.TODO(), "failed to call %s: %v", method, err)
		return fmt.Errorf("%w: %w", pam.ErrSystem, err)
	}
	if r != 0 {
		log.Debugf(context.TODO(), "failed to call %s: %s", method, pam.Error(r))
		return pam.Error(r)
	}
	return nil
}

func dbusUnsetter[K any](obj dbus.BusObject, method string, key K) error {
	var r int
	method = fmt.Sprintf("%s.%s", ifaceName, method)
	err := obj.Call(method, dbus.FlagNoAutoStart, key).Store(&r)
	if err != nil {
		log.Debugf(context.TODO(), "failed to call %s: %v", method, err)
		return fmt.Errorf("%w: %w", pam.ErrSystem, err)
	}
	if r != 0 {
		log.Debugf(context.TODO(), "failed to call %s: %s", method, pam.Error(r))
		return pam.Error(r)
	}
	return nil
}

func dbusGetter[V any, K any](obj dbus.BusObject, method string, key K) (V, error) {
	var r int
	var v V
	method = fmt.Sprintf("%s.%s", ifaceName, method)
	err := obj.Call(method, 0, key).Store(&r, &v)
	if err != nil {
		log.Debugf(context.TODO(), "failed to call %s: %v", method, err)
		return v, fmt.Errorf("%w: %w", pam.ErrSystem, err)
	}
	if r != 0 {
		log.Debugf(context.TODO(), "failed to call %s: %s", method, pam.Error(r))
		return *new(V), pam.Error(r)
	}
	return v, nil
}
