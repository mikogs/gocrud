package test

import (
	"log"
	"os"

	"go.uber.org/goleak"
)

// LeakCheckEnv is the environment variable that enables the check for leaked goroutines of CheckLeaks, when it is 1.
const LeakCheckEnv = "LEAKCHECK"

// CheckLeaks returns the exit code of the tests of a package, or 1 when the check is enabled with LeakCheckEnv and
// goroutines are still running after the tests, which it logs. Call it with the result of m.Run in TestMain, after
// closing the databases, as their connections run goroutines until they are closed.
func CheckLeaks(code int) int {
	if code != 0 || os.Getenv(LeakCheckEnv) != "1" {
		return code
	}

	if err := goleak.Find(); err != nil {
		log.Printf("Goroutines are still running after the tests: %s", err)
		return 1
	}
	return code
}
