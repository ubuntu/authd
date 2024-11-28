package types

import "encoding/json"

// LayoutType is the type of layout.
type LayoutType string

const (
	// FormLayout is a layout that displays a form.
	FormLayout LayoutType = "form"
	// QRCodeLayout is a layout that displays a QR code.
	QRCodeLayout LayoutType = "qrcode"
	// NewPasswordLayout is a layout that displays a new password form.
	NewPasswordLayout LayoutType = "new_password"

	// These are only for testing purposes, so we don't expose them publicly.
	requiredEntryLayout LayoutType = "required_entry"
	optionalEntryLayout LayoutType = "optional_entry"
)

// EntriesType is the type of entries.
type EntriesType string

const (
	// Chars is the entry value for the entries of type chars.
	Chars EntriesType = "chars"
	// CharsPassword is the entry value for entries of type chars password.
	CharsPassword EntriesType = "chars_password"
	// Digits is the entry value for entries of type digits.
	Digits EntriesType = "digits"
	// DigitsPassword is the entry value for entries of type digits password.
	DigitsPassword EntriesType = "digits_password"
)

// Layout is a UI layout for authentication.
type Layout struct {
	Type          LayoutType  `json:"type"`
	Label         string      `json:"label"`
	Button        string      `json:"button"`
	Wait          bool        `json:"wait"`
	Entry         EntriesType `json:"entry"`
	Content       string      `json:"content"`
	Code          string      `json:"code"`
	RendersQrcode bool        `json:"renders_qrcode"`
}

// LayoutFromJSON creates a Layout from JSON.
func LayoutFromJSON(data []byte) (Layout, error) {
	var layout Layout

	err := json.Unmarshal(data, &layout)
	if err != nil {
		return Layout{}, err
	}

	return layout, nil
}

// LayoutFromMap creates a Layout from a map of strings.
func LayoutFromMap(m map[string]string) (Layout, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Layout{}, err
	}

	return LayoutFromJSON(data)
}

// ToMap converts a Layout to a map of strings.
func (m *Layout) ToMap() (map[string]string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	var mp map[string]string
	err = json.Unmarshal(data, &mp)
	if err != nil {
		return nil, err
	}

	return mp, nil
}
