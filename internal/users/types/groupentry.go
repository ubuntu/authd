package types

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ubuntu/authd/internal/sliceutils"
	"github.com/ubuntu/authd/log"
)

// Validate validates the group entry values.
func (g GroupEntry) Validate() error {
	if g.Name == "" {
		return fmt.Errorf("group %q cannot have empty name", g)
	}

	if g.GID == 0 && g.Name != "root" {
		return fmt.Errorf("only root group can have GID 0, not %q", g.Name)
	}

	if strings.ContainsRune(g.Name, ',') {
		return fmt.Errorf("group %q cannot contain ',' character", g.Name)
	}

	if strings.ContainsRune(g.Passwd, ',') {
		return fmt.Errorf("group %q passwd %q cannot contain ',' character", g.Name, g.Passwd)
	}

	if slices.ContainsFunc(g.Users, func(u string) bool { return strings.ContainsRune(u, ',') }) {
		return fmt.Errorf("group %q cannot contain users with ',' character (%v)", g, g.Users)
	}

	return nil
}

// Equals checks that two groups are equal.
func (g GroupEntry) Equals(other GroupEntry) bool {
	return g.Name == other.Name &&
		g.GID == other.GID &&
		g.Passwd == other.Passwd &&
		sliceutils.EqualContent(g.Users, other.Users)
}

// DeepCopy makes a deep copy of the group entry.
func (g GroupEntry) DeepCopy() GroupEntry {
	g.Users = slices.Clone(g.Users)
	return g
}

// DeepCopyGroupEntries makes a deep copy of group entries.
func DeepCopyGroupEntries(groups []GroupEntry) []GroupEntry {
	return sliceutils.Map(groups, func(g GroupEntry) GroupEntry {
		return g.DeepCopy()
	})
}

// ValidateGroupEntries validates a list of group entries, ensuring they respect
// the [GroupEntry.Validate] constraints and that the names and the GID are unique.
func ValidateGroupEntries(groups []GroupEntry) error {
	groupNames := make(map[string]*GroupEntry, len(groups))
	groupIDs := make(map[uint32]*GroupEntry, len(groups))

	for _, g := range groups {
		if err := g.Validate(); err != nil {
			return fmt.Errorf("Group %q is not valid: %w", g, err)
		}

		if otherGroup, ok := groupNames[g.Name]; ok {
			if g.Equals(*otherGroup) {
				log.Debugf(context.Background(),
					"Skipping group %q, it's a duplicate!", g)
				continue
			}

			return fmt.Errorf("group %q is duplicate", g.Name)
		}
		if otherGroup, ok := groupIDs[g.GID]; ok {
			return fmt.Errorf("GID %d for group %q is duplicated by %q",
				g.GID, g.Name, otherGroup.Name)
		}

		groupNames[g.Name] = &g
		groupIDs[g.GID] = &g
	}

	return nil
}
