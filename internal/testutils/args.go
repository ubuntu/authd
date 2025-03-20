package testutils

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

var (
	isAsan              bool
	isAsanOnce          sync.Once
	isRace              bool
	isRaceOnce          sync.Once
	isVerbose           bool
	isVerboseOnce       sync.Once
	sleepMultiplier     float64
	sleepMultiplierOnce sync.Once
)

// IsVerbose returns whether the tests are running in verbose mode.
func IsVerbose() bool {
	isVerboseOnce.Do(func() {
		for _, arg := range os.Args {
			value, ok := strings.CutPrefix(arg, "-test.v=")
			if !ok {
				continue
			}
			isVerbose = value == "true"
		}
	})
	return isVerbose
}

func haveBuildFlag(flag string) bool {
	b, ok := debug.ReadBuildInfo()
	if !ok {
		panic("could not read build info")
	}

	flag = "-" + flag
	for _, s := range b.Settings {
		if s.Key != flag {
			continue
		}
		value, err := strconv.ParseBool(s.Value)
		if err != nil {
			panic(err)
		}
		return value
	}

	return false
}

// IsAsan returns whether the tests are running with address sanitizer.
func IsAsan() bool {
	isAsanOnce.Do(func() { isAsan = haveBuildFlag("asan") })
	return isAsan
}

// IsRace returns whether the tests are running with thread sanitizer.
func IsRace() bool {
	isRaceOnce.Do(func() { isRace = haveBuildFlag("race") })
	return isRace
}

// SleepMultiplier returns the sleep multiplier to be used in tests.
func SleepMultiplier() float64 {
	sleepMultiplierOnce.Do(func() {
		sleepMultiplier = 1
		if v := os.Getenv("AUTHD_TESTS_SLEEP_MULTIPLIER"); v != "" {
			var err error
			sleepMultiplier, err = strconv.ParseFloat(v, 64)
			if err != nil {
				panic(err)
			}
			if sleepMultiplier <= 0 {
				panic("Negative or 0 sleep multiplier is not supported")
			}
			return
		}

		if IsAsan() {
			sleepMultiplier *= 1.5
		}
		if IsRace() {
			sleepMultiplier *= 4
		}
	})

	return sleepMultiplier
}
