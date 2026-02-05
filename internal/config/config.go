package config

import (
	"os"
	"path/filepath"
	"strings"
)

type DeviceConfig struct {
	IP       string
	Password string
	Username string
}

var (
	deviceConfig *DeviceConfig
	configLoaded bool
)

func LoadDeviceConfig() (*DeviceConfig, error) {
	if deviceConfig != nil && configLoaded {
		return deviceConfig, nil
	}

	cfg := &DeviceConfig{}

	// Try to load from .env file in project root
	projectRoot := findProjectRoot()
	envPath := filepath.Join(projectRoot, ".env")

	data, err := os.ReadFile(envPath)
	if err == nil {
		parseEnvFile(string(data), cfg)
	}

	// Override with environment variables if set
	if ip := os.Getenv("DEVICE_IP"); ip != "" {
		cfg.IP = ip
	}
	if password := os.Getenv("DEVICE_PASSWORD"); password != "" {
		cfg.Password = password
	}
	if username := os.Getenv("DEVICE_USERNAME"); username != "" {
		cfg.Username = username
	}

	deviceConfig = cfg
	configLoaded = true
	return cfg, nil
}

func parseEnvFile(content string, cfg *DeviceConfig) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "DEVICE_IP":
			cfg.IP = value
		case "DEVICE_PASSWORD":
			cfg.Password = value
		case "DEVICE_USERNAME":
			cfg.Username = value
		}
	}
}

func findProjectRoot() string {
	cwd, _ := os.Getwd()
	// First check CWD for .env file
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		return cwd
	}
	// Then walk up looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return cwd
		}
		cwd = parent
	}
}

func GetDeviceIP() string {
	cfg, err := LoadDeviceConfig()
	if err != nil || cfg.IP == "" {
		return ""
	}
	return cfg.IP
}

func GetDevicePassword() string {
	cfg, err := LoadDeviceConfig()
	if err != nil || cfg.Password == "" {
		return ""
	}
	return cfg.Password
}

func GetDeviceUsername() string {
	cfg, err := LoadDeviceConfig()
	if err != nil || cfg.Username == "" {
		return ""
	}
	return cfg.Username
}

func MustGetDeviceConfig() DeviceConfig {
	cfg, err := LoadDeviceConfig()
	if err != nil {
		panic("DEVICE_IP and DEVICE_PASSWORD must be set in .env file")
	}
	if cfg.IP == "" || cfg.Password == "" {
		panic("DEVICE_IP and DEVICE_PASSWORD must be set in .env file")
	}
	return *cfg
}
