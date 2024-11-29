package types

import "strings"

const (
	// Required indicates that a layout item is required.
	Required = "required"
	// Optional indicates that a layout item is optional.
	Optional = "optional"
)

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
