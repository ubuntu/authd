package layouts

import (
	"fmt"
	"strings"

	"github.com/ubuntu/authd/internal/proto"
	"github.com/ubuntu/authd/internal/testsdetection"
)

// UIType is the type to define an authd UI layout type.
type UIType int

const (
	// UIForm is the type of a Form UI layout type.
	UIForm UIType = iota
	// UIQrCode is the type of a QrCode UI layout type.
	UIQrCode
	// UINewPassword is the type of a NewPassword UI layout type.
	UINewPassword

	// These are only for testing purposes, so we don't expose them publicly.
	uiRequiredEntry UIType = iota + 1000
	uiOptionalEntry
)

// UILayout is the type to define an authd UI layout for brokers usage.
type UILayout struct {
	*proto.UILayout
}

// UITypeError defines an error for [UIType] errors.
type UITypeError struct {
	error
}

// Is makes this error insensitive to the actual error content.
func (UITypeError) Is(err error) bool { return err == UITypeError{} }

func (u UIType) String() string {
	switch u {
	case UIForm:
		return Form
	case UIQrCode:
		return QrCode
	case UINewPassword:
		return NewPassword
	case uiRequiredEntry:
		testsdetection.MustBeTesting()
		return "required-entry"
	case uiOptionalEntry:
		testsdetection.MustBeTesting()
		return "optional-entry"
	}
	return ""
}

func uiTypeFromString(t string) (UIType, error) {
	switch t {
	case UIForm.String():
		return UIForm, nil
	case UIQrCode.String():
		return UIQrCode, nil
	case UINewPassword.String():
		return UINewPassword, nil
	case uiRequiredEntry.String():
		testsdetection.MustBeTesting()
		return uiRequiredEntry, nil
	case uiOptionalEntry.String():
		testsdetection.MustBeTesting()
		return uiOptionalEntry, nil
	}
	return UIType(-1), UITypeError{fmt.Errorf("unknown layout type %q", t)}
}

// UIOptions is the function signature used to tweak the qrcode.
type UIOptions func(*UILayout)

// WithLabel is an [UIOptions] for [NewUI] to set the label parameter in [UILayout].
func WithLabel(label string) func(l *UILayout) {
	return func(l *UILayout) { l.Label = &label }
}

// WithButton is an [UIOptions] for [NewUI] to set the button parameter in [UILayout].
func WithButton(button string) func(l *UILayout) {
	return func(l *UILayout) { l.Button = &button }
}

// WithWait is an [UIOptions] for [NewUI] to set the wait parameter in [UILayout].
func WithWait(wait string) func(l *UILayout) {
	return func(l *UILayout) { l.Wait = &wait }
}

// WithWaitBool is an [UIOptions] for [NewUI] to set the wait parameter in [UILayout] as boolean.
func WithWaitBool(wait bool) func(l *UILayout) {
	if !wait {
		return func(l *UILayout) { l.Wait = nil }
	}
	return WithWait(True)
}

// WithEntry is an [UIOptions] for [NewUI] to set the entry parameter in [UILayout].
func WithEntry(entry string) func(l *UILayout) {
	return func(l *UILayout) { l.Entry = &entry }
}

// WithContent is an [UIOptions] for [NewUI] to set the content parameter in [UILayout].
func WithContent(content string) func(l *UILayout) {
	return func(l *UILayout) { l.Content = &content }
}

// WithCode is an [UIOptions] for [NewUI] to set the code parameter in [UILayout].
func WithCode(code string) func(l *UILayout) {
	return func(l *UILayout) { l.Code = &code }
}

// WithRendersQrCode is an [UIOptions] for [NewUI] to set the qrcode rendering parameter in [UILayout].
func WithRendersQrCode(renders bool) func(l *UILayout) {
	if !renders {
		return func(l *UILayout) { l.RendersQrcode = nil }
	}
	trueStr := True
	return func(l *UILayout) { l.RendersQrcode = &trueStr }
}

// NewUI allows to create a new [UILayout] with [UIOptions].
func NewUI(t UIType, opts ...UIOptions) *UILayout {
	uiLayout := &UILayout{UILayout: &proto.UILayout{Type: t.String()}}
	for _, f := range opts {
		f(uiLayout)
	}

	return uiLayout
}

// NewUIFromMap allows to create a new [UILayout] from a map of strings how it's used in the DBus protocol.
func NewUIFromMap(layout map[string]string) (*UILayout, error) {
	uiType, err := uiTypeFromString(layout[Type])
	if err != nil {
		return nil, err
	}

	var opts []UIOptions
	if v, ok := layout[Label]; ok {
		opts = append(opts, WithLabel(v))
	}
	if v, ok := layout[Entry]; ok {
		opts = append(opts, WithEntry(v))
	}
	if v, ok := layout[Button]; ok {
		opts = append(opts, WithButton(v))
	}
	if v, ok := layout[Wait]; ok {
		opts = append(opts, WithWait(v))
	}
	if v, ok := layout[Content]; ok {
		opts = append(opts, WithContent(v))
	}
	if v, ok := layout[Code]; ok {
		opts = append(opts, WithCode(v))
	}

	if uiType != UIQrCode {
		return NewUI(uiType, opts...), nil
	}

	if v, ok := layout[RendersQrCode]; ok {
		opts = append(opts, WithRendersQrCode(v == True))
	}

	return NewUI(uiType, opts...), nil
}

// UIsFromList allows to create a new [UILayout] list from a list of map of strings
// with the format that is used in the DBus protocol.
func UIsFromList(layouts []map[string]string) ([]*UILayout, error) {
	var uiLayouts []*UILayout
	for _, l := range layouts {
		ul, err := NewUIFromMap(l)
		if err != nil {
			return nil, err
		}
		uiLayouts = append(uiLayouts, ul)
	}
	return uiLayouts, nil
}

// ToMap creates a string map from the [UILayout] that is used by DBus protocol.
func (layout UILayout) ToMap() (map[string]string, error) {
	uiType, err := uiTypeFromString(layout.Type)
	if err != nil {
		return nil, fmt.Errorf("invalid layout option: %w, got: %v", err, layout)
	}

	r := map[string]string{Type: uiType.String()}
	if l := layout.GetLabel(); l != "" {
		r[Label] = l
	}
	if b := layout.GetButton(); b != "" {
		r[Button] = b
	}
	if w := layout.GetWait(); w != "" {
		r[Wait] = w
	}
	if e := layout.GetEntry(); e != "" {
		r[Entry] = e
	}
	if c := layout.GetContent(); c != "" {
		r[Content] = c
	}
	if c := layout.GetCode(); c != "" {
		r[Code] = c
	}
	if rc := layout.GetRendersQrcode(); rc != "" {
		r[RendersQrCode] = rc
	}
	return r, nil
}

func buildItems(kind string, items ...string) string {
	return kind + ":" + strings.Join(items, ",")
}

// OptionalItems returns a formatted string of required entries.
func OptionalItems(items ...string) string {
	return buildItems(Optional, items...)
}

// RequiredItems returns a formatted string of required entries.
func RequiredItems(items ...string) string {
	return buildItems(Required, items...)
}

// ParseItems parses a string of items and returns its type and the list of items it contains.
func ParseItems(items string) (string, []string) {
	kind, items, found := strings.Cut(items, ":")
	if !found {
		return "", nil
	}

	var parsed []string
	for _, i := range strings.Split(items, ",") {
		parsed = append(parsed, strings.TrimSpace(i))
	}
	return kind, parsed
}
