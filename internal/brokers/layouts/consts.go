// Package layouts lists all the broker UI layouts we support.
package layouts

const (
	// Form is the layout used by input forms UI layouts.
	Form = "form"
	// QrCode is the layout used by device authentication UI layouts.
	QrCode = "qrcode"
	// NewPassword the layout used by new password UI layouts.
	NewPassword = "newpassword"
)

const (
	// Required indicates that a layout item is required.
	Required = "required"
	// Optional indicates that a layout item is optional.
	Optional = "optional"
)

const (
	// True is a true boolean parameter for a layout.
	True = "true"
	// False is a false boolean parameter for a layout.
	False = "false"
)

var (
	// RequiredWithBooleans indicates that a layout item is required with boolean values.
	RequiredWithBooleans = RequiredItems(True, False)
	// OptionalWithBooleans indicates that a layout item is optional with boolean values.
	OptionalWithBooleans = OptionalItems(True, False)
)
