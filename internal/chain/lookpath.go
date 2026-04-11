// internal/chain/lookpath.go
package chain

import "os/exec"

// lookPath wraps exec.LookPath so defaultLookup can be replaced in tests
// without importing os/exec throughout the package.
var lookPath = exec.LookPath
