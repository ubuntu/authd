// Package tests export cache test functionalities used by other packages.
package tests

import (
	"io"
	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolinlint as false positive then.
	_ "unsafe"

	"github.com/ubuntu/authd/internal/cache"
)

// DumpToYaml deserializes the cache database to a writer in a yaml format.
//
//go:linkname DumpToYaml github.com/ubuntu/authd/internal/cache.(*Cache).dumpToYaml
func DumpToYaml(c *cache.Cache) (string, error)

// DbfromYAML loads a yaml formatted of the buckets and dump it into destDir, with its dbname.
//
//go:linkname DbfromYAML github.com/ubuntu/authd/internal/cache.dbfromYAML
func DbfromYAML(r io.Reader, destDir string) error
