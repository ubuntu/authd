//go:build withmsentraid

package msentraid

import (
	"errors"
	"fmt"
	"math"
)

// parseUnixID converts a value read from a Graph directory extension property
// into a Unix ID. An unset value is nil.
//
// The Graph SDK stores every JSON number from a property it does not know in a
// model's additional data as a *float64, and a String or Boolean typed property
// as a *string or *bool, so those are the shapes that reach this function in
// production. The bare float64 form is here for fixtures that build additional
// data themselves.
func parseUnixID(value any) (*uint32, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case *float64:
		if value == nil {
			return nil, nil
		}
		return validateUnixID(*value)
	case float64:
		return validateUnixID(value)
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

// validateUnixID checks that a value can be a Unix ID. Directory extension
// properties hold signed 32-bit integers, so anything that is not a positive
// integer in that range is rejected.
func validateUnixID(value float64) (*uint32, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("value %v is not finite", value)
	}
	if math.Trunc(value) != value {
		return nil, fmt.Errorf("value %v is fractional", value)
	}
	if value < 0 {
		return nil, fmt.Errorf("value %v is negative", value)
	}
	if value > math.MaxInt32 {
		return nil, fmt.Errorf("value %v is greater than the signed 32-bit maximum", value)
	}

	parsed := uint32(value)
	if parsed == 0 {
		return nil, errors.New("value must be positive")
	}
	if parsed == 65534 || parsed == 65535 {
		return nil, fmt.Errorf("value %d is reserved", parsed)
	}
	return &parsed, nil
}
