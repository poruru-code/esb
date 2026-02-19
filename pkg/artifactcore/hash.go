package artifactcore

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// FileSHA256 returns the SHA-256 digest of the file content as lowercase hex.
func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
