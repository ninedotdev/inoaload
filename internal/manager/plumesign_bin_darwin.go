//go:build darwin

package manager

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed bin/plumesign
var plumesignBin []byte

var (
	plumesignPathOnce sync.Once
	plumesignPath     string
	plumesignErr      error
)

// ResolvePlumesign writes the bundled plumesign binary to a temp location on
// first call and returns its absolute path. Subsequent calls return the cached
// path. The binary is extracted once per process.
func ResolvePlumesign() (string, error) {
	plumesignPathOnce.Do(func() {
		if len(plumesignBin) == 0 {
			plumesignErr = fmt.Errorf("embedded plumesign binary is empty — rebuild with the real binary in internal/manager/bin/plumesign")
			return
		}
		dir := filepath.Join(os.TempDir(), "iNoaload")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			plumesignErr = err
			return
		}
		path := filepath.Join(dir, "plumesign")
		if err := os.WriteFile(path, plumesignBin, 0o755); err != nil {
			plumesignErr = err
			return
		}
		plumesignPath = path
	})
	return plumesignPath, plumesignErr
}
