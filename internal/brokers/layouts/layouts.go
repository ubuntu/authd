package layouts

import (
	"strings"
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
