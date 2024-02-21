//go:build with_standalone_examplebroker

package main

import (
	"context"
	"log"
	"os"

	"github.com/ubuntu/authd/examplebroker"
)

func main() {
	// Create the directory for the broker configuration files.
	cfgPath, err := os.MkdirTemp(os.TempDir(), "standalonebroker.d")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(cfgPath)

	conn, err := examplebroker.StartBus(cfgPath)
	if err != nil {
		log.Fatal("Error starting standalone broker:", err)
	}
	defer conn.Close()

	<-context.Background().Done()
}
