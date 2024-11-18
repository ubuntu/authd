package pam_test

const (
	// RunnerEnvLogFile is the environment variable used by the test client to set the log file.
	RunnerEnvLogFile = "AUTHD_PAM_RUNNER_LOG_FILE"
	// RunnerEnvSupportsConversation is the environment variable used by the test client to set whether it supports PAM conversations.
	RunnerEnvSupportsConversation = "AUTHD_PAM_RUNNER_SUPPORTS_CONVERSATION"
	// RunnerEnvExecModule is the environment variable used by the test client to set the exec module library path.
	RunnerEnvExecModule = "AUTHD_PAM_RUNNER_EXEC_MODULE"
	// RunnerEnvExecChildPath is the environment variable used by the test client to get the PAM exec child client application.
	RunnerEnvExecChildPath = "AUTHD_PAM_RUNNER_EXEC_CHILD_PATH"
	// RunnerEnvTestName is the environment variable used by the test client to set the test name.
	RunnerEnvTestName = "AUTHD_PAM_RUNNER_TEST_NAME"
	// RunnerEnvUser is the environment variable used by the test client to set the PAM user to use.
	RunnerEnvUser = "AUTHD_PAM_RUNNER_USER"
	// RunnerEnvEnvs is the environment variable used by the test client to set the PAM child environment variables.
	RunnerEnvEnvs = "AUTHD_PAM_RUNNER_ENVS"
	// RunnerEnvService is the environment variable used by the test client to set the PAM service name.
	RunnerEnvService = "AUTHD_PAM_RUNNER_SERVICE"
)

const (
	// RunnerResultActionAuthenticateFormat is the format string for Authentication action.
	RunnerResultActionAuthenticateFormat = "PAM Authenticate() for user %q"
	// RunnerResultActionChangeAuthTokFormat is the format string for ChangeAuthTok action.
	RunnerResultActionChangeAuthTokFormat = "PAM ChangeAuthTok() for user %q"
	// RunnerResultActionAcctMgmt is the string for the AcctMgmt action.
	RunnerResultActionAcctMgmt = "PAM AcctMgmt()"
)
