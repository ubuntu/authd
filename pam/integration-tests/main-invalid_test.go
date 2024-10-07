//go:build !withgdmmodel

package main_test

import (
	"log"
	"testing"
)

func TestMain(m *testing.M) {
	log.Fatal("Setup: Tests must be compiled with `withgdmmodel` tag")
}
