package users

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string

		wantID string
	}{
		"Generate ID from input":                            {input: "test", wantID: "66657"},
		"Generate ID from empty input":                      {input: "", wantID: "65536"},
		"Generate ID from input with upper case characters": {input: "TeSt", wantID: "66657"},
		"Generated ID is within the defined range":          {input: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", wantID: "71452"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantID, fmt.Sprint(GenerateID(tc.input)), "GenerateID did not return expected value")
		})
	}
}
