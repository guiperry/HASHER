package schema

import (
	"encoding/json"
)

// DocumentRecord represents a single record from the Data Miner's output in Parquet format
// This schema is used for efficient columnar storage and streaming reads
type DocumentRecord struct {
	FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
	ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
	Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
	Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

// MinedRecord represents a single record from the Data Miner's output
type MinedRecord struct {
	FileName string `json:"file_name"`
	ChunkID  int    `json:"chunk_id"`
	Content  string `json:"content"` // The actual text content to tokenize
	// Embedding field removed - will be generated on-demand via sliding windows
}

// UnmarshalJSON handles JSON unmarshaling for MinedRecord
func (mr *MinedRecord) UnmarshalJSON(data []byte) error {
	// Define a temporary type to avoid infinite recursion
	type Alias MinedRecord
	temp := &struct {
		// Embedding field kept for backward compatibility but ignored
		Embedding []interface{} `json:"embedding,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(mr),
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Embedding field is ignored - we generate embeddings on-demand via sliding windows
	_ = temp.Embedding

	return nil
}
