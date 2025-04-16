// TiCS: disabled // This is a test helper.
//go:build test_locker

package main

import (
	"log"
	"time"

	"github.com/ubuntu/authd/internal/userutils"
)

func main() {
	log.Println("Locking database...")
	err := userutils.WriteLockShadowPassword()
	if err != nil {
		log.Fatal(err)
	}

	<-time.After(999999 * time.Hour)
}
