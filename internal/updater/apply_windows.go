//go:build windows

package updater

import (
	"fmt"
	"os"
)

// applyStaged renames the running executable before installing the verified replacement.
// Windows allows renaming a loaded executable but may keep the previous file locked until exit.
func applyStaged(staged, target string) error {
	backup := target + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return fmt.Errorf("install failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	// A locked backup is harmless and will be removed before the next update attempt.
	_ = os.Remove(backup)
	return nil
}
