package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONFixing(t *testing.T) {
	// Create test cases with various JSON issues
	testCases := []struct {
		name        string
		input       string
		expectFix   bool
		shouldParse bool
	}{
		{
			name:        "Valid JSON array",
			input:       `[{"text": "hello world", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}]`,
			expectFix:   false,
			shouldParse: true,
		},
		{
			name:        "JSON array with invalid escape sequence",
			input:       `[{"text": "test\\x00value", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}]`,
			expectFix:   true,
			shouldParse: true,
		},
		{
			name:        "JSONL with valid record",
			input:       `{"text": "hello", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}`,
			expectFix:   false,
			shouldParse: true,
		},
		{
			name:        "JSONL with invalid escape sequence",
			input:       `{"text": "test\\x41world", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}`,
			expectFix:   true,
			shouldParse: true,
		},
		{
			name:        "JSONL with control characters",
			input:       `{"text": "test\u0000value", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}`,
			expectFix:   true,
			shouldParse: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the fix function
			fixed := fixJSONString(tc.input)
			actuallyFixed := fixed != tc.input

			if tc.expectFix != actuallyFixed {
				t.Errorf("Expected fix=%v, got fix=%v", tc.expectFix, actuallyFixed)
			}

			// Test if the result can be parsed
			var testRecord interface{}
			parseErr := json.Unmarshal([]byte(fixed), &testRecord)

			if tc.shouldParse && parseErr != nil {
				t.Errorf("Expected parsing to succeed, but got error: %v", parseErr)
			}

			if !tc.shouldParse && parseErr == nil {
				t.Error("Expected parsing to fail, but it succeeded")
			}

			// Log the transformation for debugging
			if actuallyFixed {
				t.Logf("Original: %s", tc.input)
				t.Logf("Fixed:    %s", fixed)
			}
		})
	}
}

func TestProcessJSONL(t *testing.T) {
	// Create a temporary file with problematic JSONL content
	tempFile := filepath.Join(t.TempDir(), "test.jsonl")

	// Test content with various issues (using actual problematic JSON)
	content := `{"text": "valid record", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test1.txt"}
{"text": "invalid` + "\x00" + `escape", "embedding": [4,5,6], "chunk_id": 2, "file_name": "test2.txt"}
{"text": "another valid", "embedding": [7,8,9], "chunk_id": 3, "file_name": "test3.txt"}
{"text": "control` + "\x01" + `char", "embedding": [10,11,12], "chunk_id": 4, "file_name": "test4.txt"}`

	err := os.WriteFile(tempFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Read and process the content
	fileContent, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Test the JSON processing logic
	lines := strings.Split(string(fileContent), "\n")
	fixedLines := []string{}
	fixedCount := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var testRecord map[string]interface{}
		if err := json.Unmarshal([]byte(line), &testRecord); err != nil {
			// Try to fix
			fixedLine := fixJSONString(line)
			if err := json.Unmarshal([]byte(fixedLine), &testRecord); err != nil {
				t.Errorf("Failed to fix unparseable line: %s", line)
				continue
			}
			fixedLines = append(fixedLines, fixedLine)
			fixedCount++
		} else {
			fixedLines = append(fixedLines, line)
		}
	}

	if fixedCount == 0 {
		t.Error("Expected some records to be fixed, but none were")
	}

	// Update the file with fixed content
	fixedContent := strings.Join(fixedLines, "\n")
	err = os.WriteFile(tempFile, []byte(fixedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write fixed content: %v", err)
	}

	// Verify the fixed content can be parsed
	updatedContent, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	updatedLines := strings.Split(string(updatedContent), "\n")
	for _, line := range updatedLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var testRecord map[string]interface{}
		if err := json.Unmarshal([]byte(line), &testRecord); err != nil {
			t.Errorf("Fixed content still has unparseable line: %s", line)
		}
	}

	t.Logf("Successfully fixed and updated %d records", fixedCount)
}

func TestJSONArrayFixing(t *testing.T) {
	// Test JSON array fixing
	content := `[{"text": "valid", "embedding": [1,2,3], "chunk_id": 1, "file_name": "test.txt"}, {"text": "invalid\\x00escape", "embedding": [4,5,6], "chunk_id": 2, "file_name": "test2.txt"}]`

	fixedContent, needsFix := fixJSONContent(content)

	if !needsFix {
		t.Error("Expected content to need fixing, but it didn't")
	}

	// Try to parse the fixed content as JSON array
	var records []interface{}
	if err := json.Unmarshal([]byte(fixedContent), &records); err != nil {
		t.Errorf("Fixed content could not be parsed as JSON array: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("Expected 2 records after fixing, got %d", len(records))
	}

	t.Logf("Successfully fixed JSON array with %d records", len(records))
}
