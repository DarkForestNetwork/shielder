// Package shversion contains version information being set via linker flags when building via the
// Makefile
package shversion

import (
	"fmt"
	"runtime"
)

var version string = "(unknown)"

// Version returns shuttermint's version string
func Version() string {
	return fmt.Sprintf("%s (%s)", version, runtime.Version())
}
