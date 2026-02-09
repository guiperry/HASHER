package factory

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadConfigFromFile loads hash method configuration from a JSON file
func LoadConfigFromFile(configPath string) (*HashMethodConfig, error) {
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultHashMethodConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var config HashMethodConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfigToFile saves hash method configuration to a JSON file
func SaveConfigToFile(config *HashMethodConfig, configPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(configPath, data, 0644)
}

// ConfigPaths returns common configuration file paths
func ConfigPaths() []string {
	homeDir, _ := os.UserHomeDir()
	return []string{
		filepath.Join(homeDir, ".hasher", "config.json"),
		"/etc/hasher/config.json",
		"./hasher-config.json",
		"./config.json",
	}
}
