package layouts_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/brokers/layouts/entries"
)

func TestOptionalItems(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		items []string

		want string
	}{
		"Optional_empty_item": {want: layouts.Optional + ":"},
		"Optional_with_one_item": {
			items: []string{entries.Chars},
			want:  layouts.Optional + ":" + entries.Chars,
		},
		"Optional_with_multiple_items": {
			items: []string{entries.Chars, entries.DigitsPassword},
			want:  layouts.Optional + ":" + entries.Chars + "," + entries.DigitsPassword,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, layouts.OptionalItems(tc.items...),
				"Unexpected optional entries item")
		})
	}
}

func TestRequiredItems(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		items []string

		want string
	}{
		"Required_empty_item": {want: layouts.Required + ":"},
		"Required_with_one_item": {
			items: []string{entries.Chars},
			want:  layouts.Required + ":" + entries.Chars,
		},
		"Required_with_multiple_items": {
			items: []string{entries.Chars, entries.DigitsPassword},
			want:  layouts.Required + ":" + entries.Chars + "," + entries.DigitsPassword,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, layouts.RequiredItems(tc.items...),
				"Unexpected required entries item")
		})
	}
}

func TestParseItems(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		items string

		wantKind  string
		wantItems []string
	}{
		"Required_empty_item": {},
		"Required_with_one_item": {
			items:     layouts.Required + ":" + entries.Chars,
			wantKind:  layouts.Required,
			wantItems: []string{entries.Chars},
		},
		"Required_with_multiple_items": {
			items:     layouts.Required + ":" + entries.Chars + ", " + entries.DigitsPassword,
			wantKind:  layouts.Required,
			wantItems: []string{entries.Chars, entries.DigitsPassword},
		},
		"Required_with_booleans": {
			items:     layouts.RequiredWithBooleans,
			wantKind:  layouts.Required,
			wantItems: []string{layouts.True, layouts.False},
		},
		"Optional_empty_item": {},
		"Optional_with_one_item": {
			items:     layouts.Optional + ":" + entries.CharsPassword,
			wantKind:  layouts.Optional,
			wantItems: []string{entries.CharsPassword},
		},
		"Optional_with_multiple_items": {
			items:     layouts.Optional + ":" + entries.Digits + " , " + entries.Chars + "," + entries.DigitsPassword,
			wantKind:  layouts.Optional,
			wantItems: []string{entries.Digits, entries.Chars, entries.DigitsPassword},
		},
		"Optional_with_booleans": {
			items:     layouts.OptionalWithBooleans,
			wantKind:  layouts.Optional,
			wantItems: []string{layouts.True, layouts.False},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kind, items := layouts.ParseItems(tc.items)
			require.Equal(t, tc.wantKind, kind, "Unexpected items kind")
			require.Equal(t, tc.wantItems, items, "Unexpected items")
		})
	}
}

func TestOptionalWithBooleans(t *testing.T) {
	t.Parallel()

	require.Equal(t, layouts.OptionalWithBooleans, fmt.Sprintf("%s:%s,%s",
		layouts.Optional, layouts.True, layouts.False), "Unexpected value")
}

func TestRequiredWithBooleans(t *testing.T) {
	t.Parallel()

	require.Equal(t, layouts.RequiredWithBooleans, fmt.Sprintf("%s:%s,%s",
		layouts.Required, layouts.True, layouts.False), "Unexpected value")
}

func TestLayoutTypes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		layout layouts.UIType

		want string
	}{
		"Empty": {
			want: layouts.Form,
		},
		"Invalid": {
			layout: layouts.UIType(-1),
		},
		"Form": {
			layout: layouts.UIForm,
			want:   layouts.Form,
		},
		"QrCode": {
			layout: layouts.UIQrCode,
			want:   layouts.QrCode,
		},
		"New password": {
			layout: layouts.UINewPassword,
			want:   layouts.NewPassword,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.layout.String(), tc.want)
		})
	}
}

func TestLayoutMap(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		layoutType layouts.UIType
		options    []layouts.UIOptions

		want      map[string]string
		wantError error
	}{
		"Empty": {
			want: map[string]string{
				layouts.Type: layouts.Form,
			},
		},
		"Password": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Gimme your password"),
				layouts.WithEntry(entries.CharsPassword),
			},
			want: map[string]string{
				layouts.Type:  layouts.Form,
				layouts.Label: "Gimme your password",
				layouts.Entry: entries.CharsPassword,
			},
		},
		"Password without wait": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Gimme your password, no wait here!"),
				layouts.WithEntry(entries.CharsPassword),
				layouts.WithWaitBool(false),
			},
			want: map[string]string{
				layouts.Type:  layouts.Form,
				layouts.Label: "Gimme your password, no wait here!",
				layouts.Entry: entries.CharsPassword,
			},
		},
		"PIN Code": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter your pin code"),
				layouts.WithEntry(entries.Digits),
			},
			want: map[string]string{
				layouts.Type:  layouts.Form,
				layouts.Label: "Enter your pin code",
				layouts.Entry: entries.Digits,
			},
		},
		"TOTP with button": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter your one time credential"),
				layouts.WithEntry(entries.Chars),
				layouts.WithButton("Resend sms"),
			},
			want: map[string]string{
				layouts.Type:   layouts.Form,
				layouts.Label:  "Enter your one time credential",
				layouts.Entry:  entries.Chars,
				layouts.Button: "Resend sms",
			},
		},
		"Fido device": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Plug your fido device and press with your thumb"),
				layouts.WithWaitBool(true),
			},
			want: map[string]string{
				layouts.Type:  layouts.Form,
				layouts.Label: "Plug your fido device and press with your thumb",
				layouts.Wait:  layouts.True,
			},
		},
		"Fido device with custom wait": {
			layoutType: layouts.UIForm,
			options: []layouts.UIOptions{
				layouts.WithLabel("Plug your fido device and press with your thumb"),
				layouts.WithWait(layouts.True),
			},
			want: map[string]string{
				layouts.Type:  layouts.Form,
				layouts.Label: "Plug your fido device and press with your thumb",
				layouts.Wait:  layouts.True,
			},
		},
		"New password": {
			layoutType: layouts.UINewPassword,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter your new password"),
				layouts.WithEntry(entries.CharsPassword),
			},
			want: map[string]string{
				layouts.Type:  layouts.NewPassword,
				layouts.Label: "Enter your new password",
				layouts.Entry: entries.CharsPassword,
			},
		},
		"New password with skip": {
			layoutType: layouts.UINewPassword,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter your new password (3 days until mandatory)"),
				layouts.WithEntry(entries.CharsPassword),
				layouts.WithButton("Skip"),
			},
			want: map[string]string{
				layouts.Type:   layouts.NewPassword,
				layouts.Label:  "Enter your new password (3 days until mandatory)",
				layouts.Entry:  entries.CharsPassword,
				layouts.Button: "Skip",
			},
		},
		"QrCode": {
			layoutType: layouts.UIQrCode,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter the following code after flashing the address:"),
			},
			want: map[string]string{
				layouts.Type:  layouts.QrCode,
				layouts.Label: "Enter the following code after flashing the address:",
			},
		},
		"QrCode with code": {
			layoutType: layouts.UIQrCode,
			options: []layouts.UIOptions{
				layouts.WithLabel("Scan the qrcode or enter the code in the login page"),
				layouts.WithCode("12345"),
				layouts.WithRendersQrCode(true),
			},
			want: map[string]string{
				layouts.Type:          layouts.QrCode,
				layouts.Label:         "Scan the qrcode or enter the code in the login page",
				layouts.Code:          "12345",
				layouts.RendersQrCode: layouts.True,
			},
		},
		"QrCode without rendering": {
			layoutType: layouts.UIQrCode,
			options: []layouts.UIOptions{
				layouts.WithLabel("Enter the code in the login page"),
				layouts.WithRendersQrCode(false),
				layouts.WithCode("12345"),
			},
			want: map[string]string{
				layouts.Type:          layouts.QrCode,
				layouts.Label:         "Enter the code in the login page",
				layouts.Code:          "12345",
				layouts.RendersQrCode: layouts.False,
			},
		},

		"Error for invalid type": {
			layoutType: layouts.UIType(-1),
			options: []layouts.UIOptions{
				layouts.WithLabel("Error Layout"),
			},
			wantError: layouts.UITypeError{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			layout := layouts.NewUI(tc.layoutType, tc.options...)
			require.NotNil(t, layout, "Setup: Layout creation failed")

			m, err := layout.ToMap()
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.want, m)

			if tc.wantError != nil {
				return
			}

			emptyValue := ""
			if layout.Label == nil {
				layout.Label = &emptyValue
			}
			if layout.Entry == nil {
				layout.Entry = &emptyValue
			}
			if layout.Button == nil {
				layout.Button = &emptyValue
			}
			if layout.Wait == nil {
				layout.Wait = &emptyValue
			}
			if layout.Content == nil {
				layout.Content = &emptyValue
			}
			if layout.Code == nil {
				layout.Code = &emptyValue
			}

			newLayout, err := layouts.NewUIFromMap(m)
			require.NoError(t, err)
			require.Equal(t, layout, newLayout)
		})
	}
}
