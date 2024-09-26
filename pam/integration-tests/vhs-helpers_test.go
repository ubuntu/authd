package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
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

	// We need to format the output a little bit, since the txt file can have some noise at the beginning.
	got := string(out)
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
	tape = []byte(fmt.Sprintf("%s\n%s", td, tape))

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
