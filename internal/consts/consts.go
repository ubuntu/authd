// Package consts defines the constants used by the project
package consts

import log "github.com/ubuntu/authd/internal/log"

var (
	// Version is the version of the executable.
	Version = "Dev"
)

const (
	// TEXTDOMAIN is the gettext domain for l10n.
	TEXTDOMAIN = "adsys"

	// DefaultLogLevel is the default logging level selected without any option.
	DefaultLogLevel = log.WarnLevel

	// DefaultSocketPath is the default socket path.
	DefaultSocketPath = "/run/authd.sock"

	// DefaultBrokersConfPath is the default configuration directory for the brokers.
	DefaultBrokersConfPath = "/etc/authd/brokers.d/"

	// DefaultCacheDir is the default directory for the cache.
	DefaultCacheDir = "/var/cache/authd/"
)
