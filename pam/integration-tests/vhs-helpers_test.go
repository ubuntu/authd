package main_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
	"github.com/ubuntu/authd/internal/testutils"
)

const (
	vhsWidth      = "Width"
	vhsHeight     = "Height"
	vhsFontFamily = "FontFamily"
	vhsFontSize   = "FontSize"
	vhsPadding    = "Padding"
	vhsMargin     = "Margin"
	vhsShell      = "Shell"
)

type tapeSetting struct {
	Key   string
	Value any
}

type tapeData struct {
	Name     string
	Outputs  []string
	Settings map[string]any
}

var (
	defaultSleepValues = map[string]time.Duration{
		"AUTHD_SLEEP_DEFAULT": 300 * time.Millisecond,
		"AUTHD_SLEEP_LONG":    1 * time.Second,
		// Keep these in sync with example broker default wait times
		"AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT":    4 * time.Second,
		"AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT": 4 * time.Second,
	}

	vhsSleepRegex = regexp.MustCompile(`(?m)\$\{?(AUTHD_SLEEP_[A-Z_]+)\}?(\s?([*/]+)\s?([\d.]+))?.*$`)
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
		Name: tapeName,
		Outputs: []string{
			tapeName + ".txt",
			// If we don't specify a .gif output, it will still create a default out.gif file.
			tapeName + ".gif",
		},
		Settings: m,
	}
}

func (td tapeData) String() string {
	var str string
	for _, o := range td.Outputs {
		str += fmt.Sprintf("Output %q\n", o)
	}
	for s, v := range td.Settings {
		str += fmt.Sprintf(`Set %s "%v"`+"\n", s, v)
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
	splitTmp := strings.Split(got, "\n")
	for i, str := range splitTmp {
		if strings.Contains(str, " ./pam_authd ") {
			got = strings.Join(splitTmp[i:], "\n")
			break
		}
	}

	return permissionstestutils.IdempotentPermissionError(got)
}

func (td tapeData) PrepareTape(t *testing.T, tapesDir, outputPath string) string {
	t.Helper()

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tape, err := os.ReadFile(filepath.Join(
		currentDir, "testdata", "tapes", tapesDir, td.Name+".tape"))
	require.NoError(t, err, "Setup: read tape file %s", td.Name)

	tapeString := evaluateTapeVariables(t, string(tape))
	tape = []byte(fmt.Sprintf("%s\n%s", td, tapeString))

	tapePath := filepath.Join(outputPath, td.Name)
	err = os.WriteFile(tapePath, tape, 0600)
	require.NoError(t, err, "Setup: write tape file")

	artifacts := []string{tapePath}
	for _, o := range td.Outputs {
		artifacts = append(artifacts, filepath.Join(outputPath, o))
	}
	saveArtifactsForDebugOnCleanup(t, artifacts)

	return tapePath
}

func evaluateTapeVariables(t *testing.T, tapeString string) string {
	t.Helper()

	for _, m := range vhsSleepRegex.FindAllStringSubmatch(tapeString, -1) {
		fullMatch, sleepKind, op, arg := m[0], m[1], m[3], m[4]
		sleep, ok := defaultSleepValues[sleepKind]
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
			fmt.Sprintf("%dms", sleepDuration(sleep).Milliseconds()))
	}

	return tapeString
}
