package main_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func prepareFileLogging(t *testing.T, fileName string) string {
	t.Helper()

	cliLog := filepath.Join(t.TempDir(), fileName)
	t.Cleanup(func() {
		out, err := os.ReadFile(cliLog)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		require.NoError(t, err, "Teardown: Impossible to read PAM client logs")
		t.Log(string(out))
	})

	return cliLog
}

func requirePreviousBrokerForUser(t *testing.T, socketPath string, brokerName string, user string) {
	t.Helper()

	conn, err := grpc.Dial(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "Can't connect to authd socket")
	t.Cleanup(func() { conn.Close() })
	pamClient := authd.NewPAMClient(conn)
	brokers, err := pamClient.AvailableBrokers(context.TODO(), nil)
	require.NoError(t, err, "Can't get available brokers")
	prevBroker, err := pamClient.GetPreviousBroker(context.TODO(), &authd.GPBRequest{Username: user})
	require.NoError(t, err, "Can't get previous broker")
	var prevBrokerID string
	for _, b := range brokers.BrokersInfos {
		if b.Name == brokerName {
			prevBrokerID = b.Id
		}
	}
	require.Equal(t, prevBroker.PreviousBroker, prevBrokerID)
}
