package idgenerator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		idMin uint32
		idMax uint32
	}{
		"Generated ID is within the defined range":        {input: "test", idMin: 1000, idMax: 2000},
		"Generate ID with minimum ID equal to maximum ID": {input: "test", idMin: 1000, idMax: 1000},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			id, err := generateID(tc.idMin, tc.idMax)
			require.NoError(t, err, "GenerateID should not have failed")

			require.GreaterOrEqual(t, id, tc.idMin, "GenerateID should return an ID greater or equal to the minimum")
			require.LessOrEqual(t, id, tc.idMax, "GenerateID should return an ID less or equal to the maximum")
		})
	}
}
