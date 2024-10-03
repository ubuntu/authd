//go:build withpamrunner

package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/pam/internal/pam_test"
	"golang.org/x/term"
)

// Simulating pam on the CLI for manual testing.
func main() {
	logFile := os.Getenv(pam_test.RunnerEnvLogFile)
	supportsConversation := os.Getenv(pam_test.RunnerEnvSupportsConversation) != ""
	execModule := os.Getenv(pam_test.RunnerEnvExecModule)
	execChildPath := os.Getenv(pam_test.RunnerEnvExecChildPath)
	testName := os.Getenv(pam_test.RunnerEnvTestName)
	pamUser := os.Getenv(pam_test.RunnerEnvUser)
	pamEnvs := os.Getenv(pam_test.RunnerEnvEnvs)
	pamService := os.Getenv(pam_test.RunnerEnvService)

	tmpDir, err := os.MkdirTemp(os.TempDir(), "pam-cli-tester-")
	if err != nil {
		log.Fatalf("Can't create temporary dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if _, err := os.Stat(execModule); err != nil {
		execModule, err = buildExecModule(tmpDir)
		if err != nil {
			log.Fatalf("Module build failed: %v", err)
		}
	}

	if _, err := os.Stat(execChildPath); err != nil {
		execChildPath, err = buildExecChild(tmpDir)
		if err != nil {
			log.Fatalf("Client build failed: %v", err)
		}
	}

	defaultArgs := []string{execChildPath, "debug=true"}
	if logFile != "" {
		defaultArgs = append(defaultArgs, "logfile="+logFile)
		defaultArgs = append(defaultArgs, "--exec-debug", "--exec-log", logFile)
	}

	if coverDir := os.Getenv("GOCOVERDIR"); coverDir != "" {
		defaultArgs = append(defaultArgs, "--exec-env", fmt.Sprintf("GOCOVERDIR=%s", coverDir))
	}

	if len(os.Args) < 2 {
		log.Fatalf("Not enough arguments")
	}

	action, args := os.Args[1], os.Args[2:]
	args = append(defaultArgs, args...)

	if pamService == "" {
		pamService = "authd-cli"
	}
	serviceFile, err := pam_test.CreateService(tmpDir, pamService, []pam_test.ServiceLine{
		{Action: pam_test.Auth, Control: pam_test.SufficientRequisite, Module: execModule, Args: args},
		{Action: pam_test.Auth, Control: pam_test.Sufficient, Module: pam_test.Ignore.String()},
		{Action: pam_test.Account, Control: pam_test.SufficientRequisite, Module: execModule, Args: args},
		{Action: pam_test.Account, Control: pam_test.Sufficient, Module: pam_test.Ignore.String()},
		{Action: pam_test.Password, Control: pam_test.SufficientRequisite, Module: execModule, Args: args},
		{Action: pam_test.Password, Control: pam_test.Sufficient, Module: pam_test.Ignore.String()},
	})
	if err != nil {
		log.Fatalf("Can't create service file %s: %v", serviceFile, err)
	}

	conversationHandler := pam.ConversationFunc(noConversationHandler)
	if supportsConversation {
		conversationHandler = pam.ConversationFunc(simpleConversationHandler)
	}

	tx, err := pam.StartConfDir(filepath.Base(serviceFile), pamUser,
		conversationHandler, filepath.Dir(serviceFile))
	if err != nil {
		log.Fatalf("Impossible to start transaction %v: %v", execChildPath, err)
	}
	defer tx.End()

	err = tx.PutEnv("AUTHD_PAM_CLI_TEST_NAME=" + testName)
	if err != nil {
		log.Fatalf("Impossible to set environment: %v", err)
	}

	if pamEnvs != "" {
		for _, env := range strings.Split(pamEnvs, ";") {
			err = tx.PutEnv(env)
			if err != nil {
				log.Fatalf("Impossible to set environment: %v", err)
			}
		}
	}

	var resultMsg string
	var pamFunc func(pam.Flags) error
	switch action {
	case "login":
		pamFunc = tx.Authenticate
		resultMsg = "PAM Authenticate() for user %q"
	case "passwd":
		pamFunc = tx.ChangeAuthTok
		resultMsg = "PAM ChangeAuthTok() for user %q"
	default:
		panic("Unknown PAM operation: " + action)
	}

	pamFlags := pam.Silent
	pamRes := pamFunc(pamFlags)
	user, _ := tx.GetItem(pam.User)

	printPamResult(fmt.Sprintf(resultMsg, user), pamRes)

	// Simulate setting auth broker as default.
	printPamResult("PAM AcctMgmt()", tx.AcctMgmt(pamFlags))
}

func noConversationHandler(style pam.Style, msg string) (string, error) {
	switch style {
	case pam.TextInfo:
		fmt.Fprintf(os.Stderr, "PAM Info Message: %s\n", msg)
	case pam.ErrorMsg:
		fmt.Fprintf(os.Stderr, "PAM Error Message: %s\n", msg)
	default:
		return "", fmt.Errorf("PAM style %d not implemented", style)
	}
	return "", nil
}

func simpleConversationHandler(style pam.Style, msg string) (string, error) {
	switch style {
	case pam.TextInfo:
		fmt.Println(msg)
	case pam.ErrorMsg:
		return noConversationHandler(style, msg)
	case pam.PromptEchoOn:
		fmt.Print(msg)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			log.Fatalf("PAM Prompt error: %v", err)
			return "", err
		}
		return strings.TrimRight(line, "\n"), nil
	case pam.PromptEchoOff:
		fmt.Print(msg)
		input, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Print("\n")
		if err != nil {
			log.Fatalf("PAM Password Prompt error: %v", err)
			return "", err
		}
		return string(input), nil
	default:
		return "", fmt.Errorf("PAM style %d not implemented", style)
	}
	return "", nil
}

func printPamResult(action string, result error) {
	var pamErr pam.Error
	if errors.As(result, &pamErr) {
		// If we got a test ignore error, then let's set it back to its actual meaning.
		if pamErr == pam_test.ErrIgnore {
			pamErr = pam.ErrIgnore
		}
		fmt.Printf("%s exited with error (PAM exit code: %d): %s\n", action, pamErr, pamErr)
		return
	}
	if result != nil {
		fmt.Printf("%s exited with error: %v\n", action, result)
		return
	}
	fmt.Printf("%s exited with success\n", action)
}

func getPkgConfigFlags(args []string) ([]string, error) {
	out, err := exec.Command("pkg-config", args...).CombinedOutput()
	if err != nil {
		fmt.Errorf("can't get pkg-config dependencies: %w: %s", err, out)
	}
	return strings.Split(strings.TrimSpace(string(out)), " "), nil
}

func buildExecModule(path string) (string, error) {
	execModule := filepath.Join(path, "pam_exec.so")
	deps, err := getPkgConfigFlags([]string{"--cflags", "--libs", "gio-2.0", "gio-unix-2.0"})
	if err != nil {
		return "", err
	}
	cmd := exec.Command("cc", "pam/go-exec/module.c", "-o", execModule,
		"-shared", "-fPIC")
	cmd.Args = append(cmd.Args, deps...)
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("can't compile exec module %s: %w\n%s", execModule, err, out)
	}

	return execModule, nil
}

func buildExecChild(path string) (string, error) {
	cliPath := filepath.Join(path, "exec-child")
	cmd := exec.Command("go", "build", "-C", "pam", "-o", cliPath)
	cmd.Dir = projectRoot()
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("can't compile child %s: %v\n%s", cliPath, err, out)
	}
	return cliPath, nil
}

// projectRoot returns the absolute path to the project root.
func projectRoot() string {
	// p is the path to the current file, in this case -> {PROJECT_ROOT}/internal/testutils/path.go
	_, p, _, _ := runtime.Caller(0)

	// Walk up the tree to get the path of the project root
	l := strings.Split(p, "/")

	// Ignores the last 4 elements -> ./pam/tools/pam-runner/pam-runner.go
	l = l[:len(l)-4]

	// strings.Split removes the first "/" that indicated an AbsPath, so we append it back in the final string.
	return "/" + filepath.Join(l...)
}
