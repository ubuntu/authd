package pam_test

const (
	// ClientEnvLogDir is the environment variable used by the test client to set the log directory.
	ClientEnvLogDir = "AUTHD_PAM_CLIENT_LOG_DIR"
	// ClientEnvSupportsConversation is the environment variable used by the test client to set whether it supports PAM conversations.
	ClientEnvSupportsConversation = "AUTHD_PAM_CLIENT_SUPPORTS_CONVERSATION"
	// ClientEnvExecModule is the environment variable used by the test client to set the exec module library path.
	ClientEnvExecModule = "AUTHD_PAM_CLIENT_EXEC_MODULE"
	// ClientEnvPath is the environment variable used by the test client to get the PAM child client application.
	ClientEnvPath = "AUTHD_PAM_CLIENT_PATH"
	// ClientEnvTestName is the environment variable used by the test client to set the test name.
	ClientEnvTestName = "AUTHD_PAM_CLIENT_TEST_NAME"
	// ClientEnvUser is the environment variable used by the test client to set the PAM user to use.
	ClientEnvUser = "AUTHD_PAM_CLIENT_USER"
	// ClientEnvEnvs is the environment variable used by the test client to set the PAM child environment variables.
	ClientEnvEnvs = "AUTHD_PAM_CLIENT_ENVS"
	// ClientEnvService is the environment variable used by the test client to set the PAM service name.
	ClientEnvService = "AUTHD_PAM_CLIENT_SERVICE"
)
