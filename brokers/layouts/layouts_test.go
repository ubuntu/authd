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
		"Optional empty item": {want: layouts.Optional + ":"},
		"Optional with one item": {
			items: []string{entries.Chars},
			want:  layouts.Optional + ":" + entries.Chars,
		},
		"Optional with multiple items": {
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
		"Required empty item": {want: layouts.Required + ":"},
		"Required with one item": {
			items: []string{entries.Chars},
			want:  layouts.Required + ":" + entries.Chars,
		},
		"Required with multiple items": {
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
		"Required empty item": {},
		"Required with one item": {
			items:     layouts.Required + ":" + entries.Chars,
			wantKind:  layouts.Required,
			wantItems: []string{entries.Chars},
		},
		"Required with multiple items": {
			items:     layouts.Required + ":" + entries.Chars + ", " + entries.DigitsPassword,
			wantKind:  layouts.Required,
			wantItems: []string{entries.Chars, entries.DigitsPassword},
		},
		"Required with booleans": {
			items:     layouts.RequiredWithBooleans,
			wantKind:  layouts.Required,
			wantItems: []string{layouts.True, layouts.False},
		},
		"Optional empty item": {},
		"Optional with one item": {
			items:     layouts.Optional + ":" + entries.CharsPassword,
			wantKind:  layouts.Optional,
			wantItems: []string{entries.CharsPassword},
		},
		"Optional with multiple items": {
			items:     layouts.Optional + ":" + entries.Digits + " , " + entries.Chars + "," + entries.DigitsPassword,
			wantKind:  layouts.Optional,
			wantItems: []string{entries.Digits, entries.Chars, entries.DigitsPassword},
		},
		"Optional with booleans": {
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
