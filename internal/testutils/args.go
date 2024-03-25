package testutils

import (
	"os"
	"strings"
	"sync"
)

var (
	isVerbose     bool
	isVerboseOnce sync.Once
)

// IsVerbose returns whether the tests are running in verbose mode.
func IsVerbose() bool {
	isVerboseOnce.Do(func() {
		for _, arg := range os.Args {
			value, ok := strings.CutPrefix(arg, "-test.v=")
			if !ok {
				continue
			}
			isVerbose = value == "true"
		}
	})
	return isVerbose
}
