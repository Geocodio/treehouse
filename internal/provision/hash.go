package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hashLockfile returns the sha256 hex digest of the main repository's copy
// of a reconcile lockfile, recorded so a later Reconcile call can detect
// drift.
func hashLockfile(mainRepository, lockfileRelativePath string) (string, error) {
	path := filepath.Join(mainRepository, lockfileRelativePath)

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hashing lockfile %s: %w", lockfileRelativePath, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hashing lockfile %s: %w", lockfileRelativePath, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
