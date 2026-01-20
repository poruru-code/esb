// Where: cli/internal/commands/interaction.go
// What: TTY detection and interactive prompts.
// Why: Support optional confirmation prompts for invalid env variables.
package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var isTerminal = func(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	// Check for character device (standard terminal detection)
	// and ensure it's not a pipe or redirect.
	return (info.Mode()&os.ModeCharDevice) != 0 && (fd == 0 || fd == 1 || fd == 2)
}

func promptYesNo(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", message)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	return trimmed == "y" || trimmed == "yes", nil
}
