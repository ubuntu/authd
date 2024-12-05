// Package sliceutils provides utility functions for slices.
package sliceutils

// Difference returns a slice with the elements that are in a but not in b.
func Difference[T comparable](a, b []T) []T {
	setB := make(map[T]struct{}, len(b))
	for _, item := range b {
		setB[item] = struct{}{}
	}

	var diff []T
	for _, item := range a {
		if _, found := setB[item]; !found {
			diff = append(diff, item)
		}
	}
	return diff
}

// Intersection returns a slice with the elements that are in both a and b.
func Intersection[T comparable](a, b []T) []T {
	setB := make(map[T]struct{}, len(b))
	for _, item := range b {
		setB[item] = struct{}{}
	}

	var intersection []T
	for _, item := range a {
		if _, found := setB[item]; found {
			intersection = append(intersection, item)
		}
	}
	return intersection
}
