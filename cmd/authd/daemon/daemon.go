// Package daemon represents the connection between the broker and pam/nss.
package daemon

import (
	"context"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/daemon"
	"github.com/ubuntu/authd/internal/services"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// cmdName is the binary name for the agent.
const cmdName = "authd"

// oldDBDir is the path of the old DB directory.
var oldDBDir = consts.OldDBDir

// App encapsulate commands and options of the daemon, which can be controlled by env variables and config files.
type App struct {
	rootCmd cobra.Command
	viper   *viper.Viper
	config  daemonConfig

	daemon *daemon.Daemon

	ready chan struct{}
}

// only overriable for tests.
type systemPaths struct {
	BrokersConf string
	Database    string
	Socket      string
}

// daemonConfig defines configuration parameters of the daemon.
type daemonConfig struct {
	Brokers     []string
	Verbosity   int
	Paths       systemPaths
	UsersConfig users.Config `mapstructure:",squash"`
}

// New registers commands and return a new App.
func New() *App {
	a := App{ready: make(chan struct{})}
	a.rootCmd = cobra.Command{
		Use:                                                                                 fmt.Sprintf("%s COMMAND", cmdName),
		Short:/*i18n.G(*/ "Authentication daemon",                                           /*)*/
		Long:/*i18n.G(*/ "Authentication daemon bridging the system with external brokers.", /*)*/
		Args:                                                                                cobra.NoArgs,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// First thing, initialize the journal handler
			log.InitJournalHandler(false)

			// Command parsing has been successful. Returns to not print usage anymore.
			a.rootCmd.SilenceUsage = true
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// TODO: before or after?  cmd.LocalFlags()

			// Set config defaults
			a.config = daemonConfig{
				Paths: systemPaths{
					BrokersConf: consts.DefaultBrokersConfPath,
					Database:    consts.DefaultDatabaseDir,
					Socket:      "",
				},
				UsersConfig: users.DefaultConfig,
			}

			// Install and unmarshall configuration
			if err := initViperConfig(cmdName, &a.rootCmd, a.viper); err != nil {
				return err
			}
			if err := a.viper.Unmarshal(&a.config); err != nil {
				return fmt.Errorf("unable to decode configuration into struct: %w", err)
			}

			setVerboseMode(a.config.Verbosity)
			log.Debugf(context.Background(), "Verbosity: %d", a.config.Verbosity)

			if err := maybeMigrateOldDBDir(oldDBDir, a.config.Paths.Database); err != nil {
				return err
			}

			if _, err := maybeMigrateBBoltToSQLite(a.config.Paths.Database); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.serve(a.config)
		},
		// We display usage error ourselves
		SilenceErrors: true,
	}
	viper := viper.New()

	a.viper = viper

	installVerbosityFlag(&a.rootCmd, a.viper)
	installConfigFlag(&a.rootCmd)

	// subcommands
	a.installVersion()

	return &a
}

// serve creates new GRPC services and listen on a TCP socket. This call is blocking until we quit it.
func (a *App) serve(config daemonConfig) error {
	ctx := context.Background()

	dbDir := config.Paths.Database
	if err := ensureDirWithPerms(dbDir, 0700); err != nil {
		close(a.ready)
		return fmt.Errorf("error initializing database directory at %q: %v", dbDir, err)
	}

	m, err := services.NewManager(ctx, dbDir, config.Paths.BrokersConf, config.Brokers, config.UsersConfig)
	if err != nil {
		close(a.ready)
		return err
	}
	// We are closing the database on exit.
	defer func() { _ = m.Stop() }()

	socketPath := config.Paths.Socket
	var daemonopts []daemon.Option
	if socketPath != "" {
		daemonopts = append(daemonopts, daemon.WithSocketPath(socketPath))
	}

	daemon, err := daemon.New(ctx, m.RegisterGRPCServices, daemonopts...)
	if err != nil {
		close(a.ready)
		return err
	}

	a.daemon = daemon
	close(a.ready)

	return daemon.Serve(ctx)
}

// installVerbosityFlag adds the -v and -vv options and returns the reference to it.
func installVerbosityFlag(cmd *cobra.Command, viper *viper.Viper) *int {
	r := cmd.PersistentFlags().CountP("verbosity", "v" /*i18n.G(*/, "issue INFO (-v), DEBUG (-vv) or DEBUG with caller (-vvv) output") //)
	decorate.LogOnError(viper.BindPFlag("verbosity", cmd.PersistentFlags().Lookup("verbosity")))
	return r
}

// Run executes the command and associated process. It returns an error on syntax/usage error.
func (a *App) Run() error {
	return a.rootCmd.Execute()
}

// UsageError returns if the error is a command parsing or runtime one.
func (a App) UsageError() bool {
	return !a.rootCmd.SilenceUsage
}

// Hup prints all goroutine stack traces and return false to signal you shouldn't quit.
func (a App) Hup() (shouldQuit bool) {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	fmt.Printf("%s", buf)
	return false
}

// Quit gracefully shutdown the service.
func (a *App) Quit() {
	a.WaitReady()
	if a.daemon == nil {
		return
	}
	a.daemon.Quit(context.Background(), false)
}

// WaitReady signals when the daemon is ready
// Note: we need to use a pointer to not copy the App object before the daemon is ready, and thus, creates a data race.
func (a *App) WaitReady() {
	<-a.ready
}

// RootCmd returns a copy of the root command for the app. Shouldn't be in general necessary apart when running generators.
func (a App) RootCmd() cobra.Command {
	return a.rootCmd
}
