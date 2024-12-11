package main_test

import (
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/proto/authd"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

const (
	vhsWidth      = "Width"
	vhsHeight     = "Height"
	vhsFontFamily = "FontFamily"
	vhsFontSize   = "FontSize"
	vhsPadding    = "Padding"
	vhsMargin     = "Margin"
	vhsShell      = "Shell"

	vhsCommandVariable = "AUTHD_TEST_TAPE_COMMAND"

	authdSleepDefault                 = "AUTHD_SLEEP_DEFAULT"
	authdSleepLong                    = "AUTHD_SLEEP_LONG"
	authdSleepCommand                 = "AUTHD_SLEEP_COMMAND"
	authdSleepExampleBrokerMfaWait    = "AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT"
	authdSleepExampleBrokerQrcodeWait = "AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT"
)

type tapeSetting struct {
	Key   string
	Value any
}

type tapeData struct {
	Name         string
	Command      string
	CommandSleep time.Duration
	Outputs      []string
	Settings     map[string]any
	Env          map[string]string
	Variables    map[string]string
}

type vhsTestType int

const (
	vhsTestTypeCLI = iota
	vhsTestTypeNative
	vhsTestTypeSSH
)

func (tt vhsTestType) tapesPath(t *testing.T) string {
	t.Helper()

	switch tt {
	case vhsTestTypeCLI:
		return "cli"
	case vhsTestTypeNative:
		return "native"
	case vhsTestTypeSSH:
		return "ssh"
	default:
		t.Errorf("Unknown test type %d", tt)
		return ""
	}
}

var (
	defaultSleepValues = map[string]time.Duration{
		authdSleepDefault: 300 * time.Millisecond,
		authdSleepCommand: 400 * time.Millisecond,
		authdSleepLong:    1 * time.Second,
		// Keep these in sync with example broker default wait times
		authdSleepExampleBrokerMfaWait:    4 * time.Second,
		authdSleepExampleBrokerQrcodeWait: 4 * time.Second,
	}

	vhsSleepRegex = regexp.MustCompile(
		`(?m)\$\{?(AUTHD_SLEEP_[A-Z_]+)\}?(\s?([*/]+)\s?([\d.]+))?(.*)$`)
	vhsEmptyLinesRegex = regexp.MustCompile(`(?m)((^\n^\n)+(^\n)?|^\n)(^─+$)`)
)

func newTapeData(tapeName string, settings ...tapeSetting) tapeData {
	m := map[string]any{
		vhsWidth:  800,
		vhsHeight: 500,
		// TODO: Ideally, we should use Ubuntu Mono. However, the github runner is still on Jammy, which does not have it.
		// We should update this to use Ubuntu Mono once the runner is updated.
		vhsFontFamily: "Monospace",
		vhsFontSize:   13,
		vhsPadding:    0,
		vhsMargin:     0,
		vhsShell:      "bash",
	}
	for _, s := range settings {
		m[s.Key] = s.Value
	}
	return tapeData{
		Name:         tapeName,
		CommandSleep: defaultSleepValues[authdSleepDefault],
		Outputs: []string{
			tapeName + ".txt",
			// If we don't specify a .gif output, it will still create a default out.gif file.
			tapeName + ".gif",
		},
		Settings: m,
		Env:      make(map[string]string),
	}
}

type clientOptions struct {
	PamUser        string
	PamEnv         []string
	PamServiceName string
	Term           string
	SessionType    string
}

func (td *tapeData) AddClientOptions(t *testing.T, opts clientOptions) {
	t.Helper()

	logFile := prepareFileLogging(t, "authd-pam-test-client.log")
	td.Env[pam_test.RunnerEnvLogFile] = logFile
	td.Env[pam_test.RunnerEnvTestName] = t.Name()

	if opts.PamUser != "" {
		td.Env[pam_test.RunnerEnvUser] = opts.PamUser
	}
	if opts.PamEnv != nil {
		td.Env[pam_test.RunnerEnvEnvs] = strings.Join(opts.PamEnv, ";")
	}
	if opts.PamServiceName != "" {
		td.Env[pam_test.RunnerEnvService] = opts.PamServiceName
	}
	if opts.Term != "" {
		td.Env["AUTHD_PAM_CLI_TERM"] = opts.Term
	}
	if opts.SessionType != "" {
		td.Env["XDG_SESSION_TYPE"] = opts.SessionType
	}
}

func (td tapeData) RunVhs(t *testing.T, testType vhsTestType, outDir string, cliEnv []string) {
	t.Helper()

	cmd := exec.Command("env", "vhs")
	cmd.Env = append(testutils.AppendCovEnv(cmd.Env), cliEnv...)
	cmd.Dir = outDir

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	cmd.Env = append(cmd.Env, prependBinToPath(t))

	u, err := user.Current()
	require.NoError(t, err, "Setup: getting current user")
	if u.Name == "root" || os.Getenv("SCHROOT_CHROOT_NAME") != "" {
		cmd.Env = append(cmd.Env, "VHS_NO_SANDBOX=1")
	}

	// Move some of the environment specific-variables from the tape to the launched process
	if e, ok := td.Env[pam_test.RunnerEnvLogFile]; ok {
		delete(td.Env, pam_test.RunnerEnvLogFile)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", pam_test.RunnerEnvLogFile, e))
	}

	cmd.Args = append(cmd.Args, td.PrepareTape(t, testType, outDir))
	out, err := cmd.CombinedOutput()

	isSSHError := func(processOut []byte) bool {
		const sshConnectionResetByPeer = "Connection reset by peer"
		const sshConnectionClosed = "Connection closed by"
		output := string(processOut)
		return strings.Contains(output, sshConnectionResetByPeer) ||
			strings.Contains(output, sshConnectionClosed)
	}
	if err != nil && testType == vhsTestTypeSSH && isSSHError(out) {
		t.Logf("SSH Connection failed on tape %q: %v: %s", td.Name, err, out)
		// We've sometimes (but rarely) seen SSH connection errors which were resolved on retry, so we retry once.
		// If it fails again, something might actually be broken.
		//nolint:gosec // G204 it's a test and we explicitly set the parameters before.
		newCmd := exec.Command(cmd.Args[0], cmd.Args[1:]...)
		newCmd.Dir = cmd.Dir
		newCmd.Env = slices.Clone(cmd.Env)
		out, err = newCmd.CombinedOutput()
	}
	require.NoError(t, err, "Failed to run tape %q: %v: %s", td.Name, err, out)
}

func (td tapeData) String() string {
	var str string
	for _, o := range td.Outputs {
		str += fmt.Sprintf("Output %q\n", o)
	}
	for s, v := range td.Settings {
		str += fmt.Sprintf(`Set %s "%v"`+"\n", s, v)
	}
	for s, v := range td.Env {
		str += fmt.Sprintf(`Env %s %q`+"\n", s, v)
	}
	return str
}

func (td tapeData) Output() string {
	var txt string
	for _, o := range td.Outputs {
		if strings.HasSuffix(o, ".txt") {
			txt = o
		}
	}
	return txt
}

func (td tapeData) ExpectedOutput(t *testing.T, outputDir string) string {
	t.Helper()

	outPath := filepath.Join(outputDir, td.Output())
	out, err := os.ReadFile(outPath)
	require.NoError(t, err, "Could not read output file of tape %q (%s)", td.Name, outPath)
	got := string(out)

	if testutils.IsRace() && strings.Contains(got, "WARNING: DATA RACE") &&
		strings.Contains(got, "bubbles/cursor.(*Model).BlinkCmd.func1") {
		// FIXME: This is a well known race of bubble tea:
		// https://github.com/charmbracelet/bubbletea/issues/909
		// We can't do much here, as the workaround will likely affect the
		// GUI behavior, but we ignore this since it's definitely not our bug.
		t.Skip("This is a very well known bubble tea bug (#909), ignoring it")
		if testutils.IsVerbose() {
			t.Logf("Ignored bubbletea race:\n%s", got)
		} else {
			fmt.Fprintf(os.Stderr, "Ignored bubbletea race:\n%s", got)
		}
	}

	// We need to format the output a little bit, since the txt file can have some noise at the beginning.
	command := "> " + td.Command
	maxCommandLen := 0
	splitTmp := strings.Split(got, "\n")
	for _, str := range splitTmp {
		maxCommandLen = max(maxCommandLen, utf8.RuneCountInString(str))
	}
	if len(command) > maxCommandLen {
		command = command[:maxCommandLen]
	}
	for i, str := range splitTmp {
		if strings.Contains(str, command) {
			got = strings.Join(splitTmp[i:], "\n")
			break
		}
	}

	got = permissionstestutils.IdempotentPermissionError(got)

	// Drop all the empty lines before each page separator, to remove the clutter.
	got = vhsEmptyLinesRegex.ReplaceAllString(got, "$4")

	// Save the sanitized result on cleanup
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		baseName, _ := strings.CutSuffix(td.Output(), ".txt")
		tempOutput := filepath.Join(t.TempDir(), fmt.Sprintf("%s_sanitized.txt", baseName))
		require.NoError(t, os.WriteFile(tempOutput, []byte(got), 0600),
			"TearDown: Saving sanitized output file %q", tempOutput)
		saveArtifactsForDebug(t, []string{tempOutput})
	})

	return got
}

func (td tapeData) PrepareTape(t *testing.T, testType vhsTestType, outputPath string) string {
	t.Helper()

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tape, err := os.ReadFile(filepath.Join(
		currentDir, "testdata", "tapes", testType.tapesPath(t), td.Name+".tape"))
	require.NoError(t, err, "Setup: read tape file %s", td.Name)

	tapeString := evaluateTapeVariables(t, string(tape), td)

	tapeLines := strings.Split(tapeString, "\n")
	var lastCommand string
	for i := len(tapeLines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(tapeLines[i])
		if len(s) == 0 || s[0] == '#' {
			continue
		}
		if idx := strings.Index(s, "#"); idx > 0 {
			s = strings.TrimSpace(s[:idx])
		}
		lastCommand = s
		break
	}
	require.Equal(t, "Show", lastCommand,
		"Setup: Tape %q must terminate with a `Show` command", td.Name)

	tape = []byte(strings.Join([]string{
		td.String(),
		tapeString,
		// Note that not sleeping enough may lead to a system hang, so keep it
		// in mind if tests are failing in CI with with error code 143.
		fmt.Sprintf("Sleep %dms",
			sleepDuration(defaultSleepValues[authdSleepDefault]).Milliseconds()),
	}, "\n"))

	tapePath := filepath.Join(outputPath, td.Name)
	err = os.WriteFile(tapePath, tape, 0600)
	require.NoError(t, err, "Setup: write tape file")

	if testutils.IsVerbose() {
		t.Logf("Tape %q is now:\n%s", td.Name, tape)
	}

	artifacts := []string{tapePath}
	for _, o := range td.Outputs {
		artifacts = append(artifacts, filepath.Join(outputPath, o))
	}
	saveArtifactsForDebugOnCleanup(t, artifacts)

	return tapePath
}

func evaluateTapeVariables(t *testing.T, tapeString string, td tapeData) string {
	t.Helper()

	require.Greater(t, td.CommandSleep.Milliseconds(), int64(0),
		"Setup: Command sleep can't be unset or minor than 0")

	sleepValues := &defaultSleepValues
	if td.CommandSleep != defaultSleepValues[authdSleepCommand] {
		sv := maps.Clone(*sleepValues)
		sv[authdSleepCommand] = td.CommandSleep
		sleepValues = &sv
	}

	for _, m := range vhsSleepRegex.FindAllStringSubmatch(tapeString, -1) {
		fullMatch, sleepKind, op, arg, rest := m[0], m[1], m[3], m[4], m[5]
		sleep, ok := (*sleepValues)[sleepKind]
		require.True(t, ok, "Setup: unknown sleep kind: %q", sleepKind)

		// We don't need to support math that is complex enough to use proper parsers as go.ast
		if arg != "" {
			parsedArg, err := strconv.ParseFloat(arg, 32)
			require.NoError(t, err, "Setup: Cannot parse expression %q: %q is not a float", fullMatch, arg)

			switch op {
			case "*":
				sleep = time.Duration(math.Round(float64(sleep) * parsedArg))
			case "/":
				require.NotZero(t, parsedArg, "Setup: Division by zero")
				sleep = time.Duration(math.Round(float64(sleep) / parsedArg))
			default:
				require.Empty(t, op, "Setup: Unhandled operator %q", op)
			}
		}

		replaceRegex := regexp.MustCompile(fmt.Sprintf(`(?m)%s$`, regexp.QuoteMeta(fullMatch)))
		tapeString = replaceRegex.ReplaceAllString(tapeString,
			fmt.Sprintf("%dms%s", sleepDuration(sleep).Milliseconds(), rest))
	}

	if td.Command == "" {
		require.NotContains(t, tapeString, fmt.Sprintf("${%s}", vhsCommandVariable),
			"Setup: Tape contains %q but it's not defined", vhsCommandVariable)
	}

	variables := td.Variables
	if td.Command != "" {
		if variables != nil {
			variables = maps.Clone(variables)
		}
		if variables == nil {
			variables = make(map[string]string)
		}
		variables[vhsCommandVariable] = td.Command
	}

	for k, v := range variables {
		variable := fmt.Sprintf("${%s}", k)
		require.Contains(t, tapeString, variable,
			"Setup: Tape does not contain %q", variable)
		tapeString = strings.ReplaceAll(tapeString, variable, v)
	}

	return tapeString
}

func requireRunnerResultForUser(t *testing.T, sessionMode authd.SessionMode, user, goldenContent string) {
	t.Helper()

	// Only check the last 50 lines of the golden file, because that's where
	// the result is printed, while printing the full output on failure is too much.
	goldenLines := strings.Split(goldenContent, "\n")
	goldenContent = strings.Join(goldenLines[max(0, len(goldenLines)-50):], "\n")

	require.Contains(t, goldenContent, pam_test.RunnerAction(sessionMode).Result().Message(user),
		"Golden file does not include required value, consider increasing the terminal size:\n%s",
		goldenContent)
	require.Contains(t, goldenContent, pam_test.RunnerResultActionAcctMgmt.Message(user),
		"Golden file does not include required value, consider increasing the terminal size:\n%s",
		goldenContent)
}

func requireRunnerResult(t *testing.T, sessionMode authd.SessionMode, goldenContent string) {
	t.Helper()

	requireRunnerResultForUser(t, sessionMode, "", goldenContent)
}
