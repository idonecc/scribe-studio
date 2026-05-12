package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
)

// ErrBinaryMissing means the requested tool couldn't be found anywhere we
// looked. The pipeline surfaces this to the UI so the user can install it.
var ErrBinaryMissing = errors.New("binary not found")

// BinaryPath resolves `name` against, in order:
//  1. resources/bin/<name>  (co-located with the running executable — how
//     the production .app bundle ships)
//  2. resources/bin/<os>-<arch>/<name>  (per-platform subdir for release
//     builds that want to ship more than one arch)
//  3. resources/bin/<name>  (relative to the project root when running
//     `wails dev` from source; fetch-bins.sh symlinks to brew here)
//  4. the user's PATH  (ultimate fallback)
// Returns an absolute path or ErrBinaryMissing.
func BinaryPath(name string) (string, error) {
	if goruntime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	archTag := fmt.Sprintf("%s-%s", goruntime.GOOS, goruntime.GOARCH)

	candidates := []string{}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		// macOS .app bundle: <App>.app/Contents/MacOS/<exe>
		// Resources sit at  <App>.app/Contents/Resources/bin/<name>.
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "Resources", "bin", archTag, name),
			filepath.Join(exeDir, "..", "Resources", "bin", name),
			filepath.Join(exeDir, "resources", "bin", archTag, name),
			filepath.Join(exeDir, "resources", "bin", name),
		)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "resources", "bin", archTag, name),
			filepath.Join(cwd, "resources", "bin", name),
		)
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}

	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("%w: %s", ErrBinaryMissing, name)
}
