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
		"test_difference_between_two_slices": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{3, 4, 5, 6, 7},
			want: []int{1, 2},
		},
		"test_difference_between_an_empty_slice_and_a_non-empty_slice": {
			a:    []int{},
			b:    []int{3, 4, 5, 6, 7},
			want: []int(nil),
		},
		"test_difference_between_a_non-empty_slice_and_an_empty_slice": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{},
			want: []int{1, 2, 3, 4, 5},
		},
		"test_difference_between_two_empty_slices": {
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
		"test_intersection_between_two_slices": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{3, 4, 5, 6, 7},
			want: []int{3, 4, 5},
		},
		"test_intersection_between_an_empty_slice_and_a_non-empty_slice": {
			a:    []int{},
			b:    []int{3, 4, 5, 6, 7},
			want: []int(nil),
		},
		"test_intersection_between_a_non-empty_slice_and_an_empty_slice": {
			a:    []int{1, 2, 3, 4, 5},
			b:    []int{},
			want: []int(nil),
		},
		"test_intersection_between_two_empty_slices": {
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
