package auth

import (
	"fmt"

	"github.com/ubuntu/authd/brokers/layouts"
)

// InvalidModeError defines an error for invalid [Mode] errors.
type InvalidModeError struct {
	Message string
}

// Error is the implementation of the error interface.
func (e InvalidModeError) Error() string {
	return e.Message
}

// Is makes this error insensitive to the actual error content.
func (InvalidModeError) Is(err error) bool { return err == InvalidModeError{} }

// ModeOptions is the function signature used to tweak the qrcode.
type ModeOptions func(*Mode)

// NewMode allows to create a new [Mode] with [ModeOptions].
func NewMode(id, label string, opts ...ModeOptions) Mode {
	mode := Mode{Id: id, Label: label}
	for _, opt := range opts {
		opt(&mode)
	}

	return mode
}

// ToMap creates a string map from the [Mode] that is used by DBus protocol.
func (mode Mode) ToMap() (map[string]string, error) {
	if mode.Id == "" {
		return nil, InvalidModeError{"invalid empty mode ID"}
	}
	if mode.Label == "" {
		return nil, InvalidModeError{"invalid empty mode label"}
	}

	return map[string]string{
		layouts.ID:    mode.Id,
		layouts.Label: mode.Label,
	}, nil
}

// NewModeFromMap allows to create a new [Mode] from a map of strings how it's used in the DBus protocol.
func NewModeFromMap(mode map[string]string) (Mode, error) {
	id := mode[layouts.ID]
	label := mode[layouts.Label]

	if id == "" {
		return Mode{}, InvalidModeError{
			fmt.Sprintf("invalid authentication mode, missing %q key: %v", layouts.ID, mode),
		}
	}
	if label == "" {
		return Mode{}, InvalidModeError{
			fmt.Sprintf("invalid authentication mode, missing %q key: %v", layouts.Label, mode),
		}
	}

	return NewMode(id, label), nil
}

// NewModeMaps creates a list of string maps from the list of [Mode] how it's used by DBus protocol.
func NewModeMaps(modes []Mode) ([]map[string]string, error) {
	var maps []map[string]string

	for _, m := range modes {
		m, err := m.ToMap()
		if err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	return maps, nil
}
