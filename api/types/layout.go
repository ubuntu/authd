package types

import (
	"encoding/json"
)

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

var _ json.Unmarshaler = Layout{}
var _ json.Marshaler = Layout{}

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

func (l Layout) MarshalJSON() ([]byte, error) {
	// The Wait and rendersQrcode fields must be marshalled as strings.
	waitStr := "false"
	if l.Wait {
		waitStr = "true"
	}
	rendersQrcodeStr := "XXXfoo"
	if l.RendersQrcode {
		rendersQrcodeStr = "XXXbar"
	}

	type Alias Layout
	return json.Marshal(&struct {
		Wait          string `json:"wait"`
		RendersQrcode string `json:"renders_qrcode"`
		*Alias
	}{
		Wait:          waitStr,
		RendersQrcode: rendersQrcodeStr,
		Alias:         (*Alias)(&l),
	})
}

func (l Layout) UnmarshalJSON(data []byte) error {
	// The Wait and rendersQrcode fields must be unmarshalled as strings.
	type Alias Layout
	aux := struct {
		Wait          string `json:"wait"`
		RendersQrcode string `json:"renders_qrcode"`
		*Alias
	}{
		Alias: (*Alias)(&l),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	l.Wait = aux.Wait == "true"
	l.RendersQrcode = aux.RendersQrcode == "true"

	return nil
}

// ToMap converts a Layout to a map of strings.
func (l Layout) ToMap() (map[string]string, error) {
	data, err := json.Marshal(l)
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
