package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/brokers/auth"
	"github.com/ubuntu/authd/brokers/layouts"
)

func TestAuthModeMap(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		id    string
		label string

		want      map[string]string
		wantError error
	}{
		"Simple": {
			id:    "some-id",
			label: "Some Label",
			want: map[string]string{
				layouts.ID:    "some-id",
				layouts.Label: "Some Label",
			},
		},

		// Error cases.
		"Error on Empty ID": {
			wantError: auth.InvalidModeError{},
		},
		"Error on Empty Label": {
			id:        "some-id",
			wantError: auth.InvalidModeError{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mode := auth.NewMode(tc.id, tc.label)
			require.NotNil(t, mode, "Setup: Mode creation failed")

			m, err := mode.ToMap()
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.want, m)

			if tc.wantError != nil {
				return
			}

			newMode, err := auth.NewModeFromMap(m)
			require.NoError(t, err)
			require.Equal(t, mode, newMode)
		})
	}
}

func TestAuthModesFromMapErrors(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		id        string
		wantError error
	}{
		"Error on Empty ID": {},
		"Error on Empty Label": {
			id: "some-id",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			newMode, err := auth.NewModeFromMap(map[string]string{
				layouts.ID: tc.id,
			})
			require.Nil(t, newMode, "Mode should be unset")
			require.ErrorIs(t, err, auth.InvalidModeError{})
		})
	}
}

func TestModeList(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		modes []*auth.Mode

		want      []map[string]string
		wantError error
	}{
		"Empty": {},
		"Single mode": {
			modes: []*auth.Mode{
				auth.NewMode("some-id", "Some Label"),
			},
			want: []map[string]string{
				{
					layouts.ID:    "some-id",
					layouts.Label: "Some Label",
				},
			},
		},
		"Multiple modes": {
			modes: []*auth.Mode{
				auth.NewMode("some-id", "Some Label"),
				auth.NewMode("some-other-id", "Some Other Label"),
			},
			want: []map[string]string{
				{
					layouts.ID:    "some-id",
					layouts.Label: "Some Label",
				},
				{
					layouts.ID:    "some-other-id",
					layouts.Label: "Some Other Label",
				},
			},
		},

		// Error cases
		"Error for invalid ID": {
			modes: []*auth.Mode{
				auth.NewMode("some-id", "Some Label"),
				auth.NewMode("", ""),
				auth.NewMode("some-other-id", "Some Other Label"),
			},
			wantError: auth.InvalidModeError{},
		},
		// Error cases
		"Error for invalid Label": {
			modes: []*auth.Mode{
				auth.NewMode("some-id", "Some Label"),
				auth.NewMode("some-id", ""),
				auth.NewMode("some-other-id", "Some Other Label"),
			},
			wantError: auth.InvalidModeError{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			maps, err := auth.NewModeMaps(tc.modes)
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.want, maps)
		})
	}
}
