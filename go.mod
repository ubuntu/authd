module github.com/ubuntu/authd

go 1.23.0

toolchain go1.23.6

require (
	github.com/charmbracelet/bubbles v0.20.0
	github.com/charmbracelet/bubbletea v1.3.3
	github.com/charmbracelet/lipgloss v1.0.0
	github.com/charmbracelet/x/term v0.2.1
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/godbus/dbus/v5 v5.1.0
	github.com/google/uuid v1.6.0
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/msteinert/pam/v2 v2.0.0
	github.com/muesli/termenv v0.15.2
	github.com/otiai10/copy v1.14.1
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/spf13/cobra v1.9.1
	github.com/spf13/viper v1.19.0
	github.com/stretchr/testify v1.10.0
	github.com/ubuntu/decorate v0.0.0-20230606064312-bc4ac83958d6
	go.etcd.io/bbolt v1.4.0
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9
	golang.org/x/sys v0.30.0
	golang.org/x/term v0.29.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/yaml.v3 v3.0.1
	gorbe.io/go/osrelease v0.3.0
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sahilm/fuzzy v0.1.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a // indirect
)

// FIXME: Use released version once we have one!
// The branch below includes changes from this upstream PR:
// - https://github.com/msteinert/pam/pull/13
replace github.com/msteinert/pam/v2 => github.com/3v1n0/go-pam/v2 v2.0.0-20240321054421-f19903865176
