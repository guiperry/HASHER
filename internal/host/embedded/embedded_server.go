package embedded

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed hasher-server-mips
var embeddedServerContent []byte

// GetEmbeddedServerBinary retrieves the content of the embedded hasher-server-mips binary.
func GetEmbeddedServerBinary() ([]byte, error) {
	if len(embeddedServerContent) == 0 {
		return nil, fmt.Errorf("hasher-server-mips binary not embedded or empty")
	}
	return embeddedServerContent, nil
}

// ExtractEmbeddedServerBinary extracts the embedded hasher-server-mips binary to a specified target path.
func ExtractEmbeddedServerBinary(targetPath string) error {
	data, err := GetEmbeddedServerBinary()
	if err != nil {
		return err
	}

	// Write the embedded binary content to the target path
	if err := os.WriteFile(targetPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write embedded hasher-server-mips to %s: %w", targetPath, err)
	}
	return nil
}
