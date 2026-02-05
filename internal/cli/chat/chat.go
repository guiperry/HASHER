package chat

import (
	"os"
	"os/exec"
	"strings"
)

func ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ExecuteSudoCommand(command string) (string, error) {
	cmd := exec.Command("sudo", "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func ParseInput(input string) (string, string, bool) {
	if strings.HasPrefix(input, "/quit") {
		return "", "", true
	}

	if strings.HasPrefix(input, "/reset") {
		return "/reset", "", false
	}

	if strings.HasPrefix(input, "/file ") {
		return "/file", strings.TrimSpace(strings.TrimPrefix(input, "/file ")), false
	}

	if strings.HasPrefix(input, "/sudo ") {
		return "/sudo", strings.TrimSpace(strings.TrimPrefix(input, "/sudo ")), false
	}

	return "", input, false
}
