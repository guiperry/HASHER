package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	// Default input path
	homeDir, _ := os.UserHomeDir()
	inputFile := filepath.Join(homeDir, ".local", "share", "dataminer", "json", "ai_knowledge_base.json")

	// Read and parse the JSON to see the actual structure
	content, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Printf("Failed to read file: %v\n", err)
		return
	}

	// Fix the JSON first
	fixed := fixJSONString(string(content))
	if !strings.HasSuffix(strings.TrimSpace(fixed), "]") {
		fixed = fixed + "\n]"
	}

	var records []interface{}
	if err := json.Unmarshal([]byte(fixed), &records); err != nil {
		fmt.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	if len(records) > 0 {
		// Show the structure of the first record
		record, ok := records[0].(map[string]interface{})
		if !ok {
			fmt.Printf("Record is not an object\n")
			return
		}

		fmt.Printf("Record structure:\n")
		for key, value := range record {
			switch v := value.(type) {
			case string:
				if len(v) > 100 {
					fmt.Printf("  %s: (string, length %d) %.100q...\n", key, len(v), v)
				} else {
					fmt.Printf("  %s: (string) %q\n", key, v)
				}
			case []float32:
				fmt.Printf("  %s: ([]float32, length %d)\n", key, len(v))
			case []interface{}:
				fmt.Printf("  %s: ([]interface{}, length %d)\n", key, len(v))
			default:
				fmt.Printf("  %s: (%T) %v\n", key, v, key)
			}
		}
	}
}

func fixJSONString(jsonStr string) string {
	re := regexp.MustCompile(`\\x[0-9a-fA-F]{2}`)
	fixed := re.ReplaceAllString(jsonStr, "")

	re = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F]`)
	fixed = re.ReplaceAllString(fixed, "")

	re = regexp.MustCompile(`\\[^"\\bfnrt/]`)
	fixed = re.ReplaceAllString(fixed, "")

	return fixed
}
