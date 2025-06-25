// TiCS: disabled // This is a test helper.
//go:build test_locker

package main

import (
	"log"
	"time"

	userslocking "github.com/ubuntu/authd/internal/users/locking"
)

func main() {
	log.Println("Locking database...")
	err := userslocking.WriteRecLock()
	if err != nil {
		log.Fatal(err)
	}

	<-time.After(999999 * time.Hour)
}
