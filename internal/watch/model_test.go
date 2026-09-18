//go:build modeltest

package watch

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed model.pml
var modelFile string

// TestWatchModel uses the [Spin] tool to validate the formal Promela model of
// the Hypcast watch implementation in model.pml.
//
// To verify a Promela model, a user will typically:
//
//   - Use Spin to create a C program ("pan.c") that evaluates all possible
//     states in the model.
//   - Compile this program with the system C compiler, and execute it.
//   - Look for the presence of a ".trail" file demonstrating an interleaving of
//     processes that violates the model's assertions.
//
// This harness automates this process within the standard Go testing framework,
// building the model checker into a test artifact directory and displaying the
// details of any generated trail.
//
// Use the "modeltest" build tag to include this harness in a test run, and
// include the "-v" flag to display the model checker's output:
//
//	go test ./internal/watch -v -tags modeltest -run Model
//
// Include the "-artifacts" flag to persist the "pan" binary and any "*.trail"
// file rather than let Go's test framework delete them automatically.
//
// [Spin]: https://spinroot.com/
func TestModel(t *testing.T) {
	for _, cmd := range []string{"spin", "cc"} {
		_, err := exec.LookPath(cmd)
		require.NoError(t, err, "%v must be installed to run model tests", cmd)
	}

	t.Chdir(t.ArtifactDir())

	spin := exec.Command("spin", "-a", "/dev/stdin")
	spin.Stdin = strings.NewReader(modelFile)
	spin.Stdout, spin.Stderr = os.Stdout, os.Stderr
	err := spin.Run()
	require.NoError(t, err, "spin should run without errors")

	cc := exec.Command("cc", "-o", "pan", "pan.c")
	cc.Stdout, cc.Stderr = os.Stdout, os.Stderr
	err = cc.Run()
	require.NoError(t, err, "pan.c should compile without errors")

	pan := exec.Command(filepath.Join(t.ArtifactDir(), "pan"))
	pan.Stdout, pan.Stderr = os.Stdout, os.Stderr
	err = pan.Run()
	require.NoError(t, err, "pan should run without errors")

	matches, _ := filepath.Glob("*.trail") // Error-free for well-formed patterns.
	if len(matches) == 0 {
		return
	}

	trail := exec.Command("spin", "-t", "-p", "-k", matches[0], "/dev/stdin")
	trail.Stdin = strings.NewReader(modelFile)
	trail.Stdout, trail.Stderr = os.Stdout, os.Stderr
	err = trail.Run()
	if assert.NoError(t, err, "spin should print the trail without errors") {
		assert.Fail(t, "Model shouldn't produce a *.trail file")
	}
}
