package sliceutils_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/sliceutils"
)

func TestDifference(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		a, b, want []int
	}{
		"test difference between two slices": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{3, 4, 5, 6, 7},
			want: []int{1, 2},
		},
		"test difference between an empty slice and a non-empty slice": {
			a:    []int{},
			b:    []int{3, 4, 5, 6, 7},
			want: []int(nil),
		},
		"test difference between a non-empty slice and an empty slice": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{},
			want: []int{1, 2, 3, 4, 5},
		},
		"test difference between two empty slices": {
			a:    []int{},
			b:    []int{},
			want: []int(nil),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := sliceutils.Difference(tc.a, tc.b)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIntersection(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		a, b, want []int
	}{
		"test intersection between two slices": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{3, 4, 5, 6, 7},
			want: []int{3, 4, 5},
		},
		"test intersection between an empty slice and a non-empty slice": {
			a:    []int{},
			b:    []int{3, 4, 5, 6, 7},
			want: []int(nil),
		},
		"test intersection between a non-empty slice and an empty slice": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{},
			want: []int(nil),
		},
		"test intersection between two empty slices": {
			a:    []int{},
			b:    []int{},
			want: []int(nil),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := sliceutils.Intersection(tc.a, tc.b)
			require.Equal(t, tc.want, got)
		})
	}
}
