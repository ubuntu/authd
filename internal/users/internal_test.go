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
		idMin uint32
		idMax uint32

		wantID string
	}{
		"Generate ID from input":                            {input: "test", wantID: "1190748311"},
		"Generate ID from empty input":                      {input: "", wantID: "1820012610"},
		"Generate ID from input with upper case characters": {input: "TeSt", wantID: "1190748311"},
		"Generated ID is within the defined range":          {input: "test", idMin: 1000, idMax: 2000, wantID: "1008"},
		"Generate ID with minimum ID equal to maximum ID":   {input: "test", idMin: 1000, idMax: 1000, wantID: "1000"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.idMin == 0 {
				tc.idMin = DefaultConfig.UIDMin
			}
			if tc.idMax == 0 {
				tc.idMax = DefaultConfig.UIDMax
			}

			require.Equal(t, tc.wantID, fmt.Sprint(generateID(tc.input, tc.idMin, tc.idMax)),
				"GenerateID did not return expected value")
		})
	}
}
