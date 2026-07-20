//go:build !windows

package updater

import "os"

// applyStaged uses an atomic rename on Unix after the archive has been fully verified.
func applyStaged(staged, target string) error {
	return os.Rename(staged, target)
}
