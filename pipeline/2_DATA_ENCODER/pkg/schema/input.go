package schema

import (
	"encoding/json"
)

// DocumentRecord represents a single record from the Data Miner's output in Parquet format
type DocumentRecord struct {
	FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
	ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
	Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
	Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`

	// NLP Metadata
	Tokens       []string `parquet:"name=tokens, type=LIST, valuetype=BYTE_ARRAY, valueconvertedtype=UTF8"`
	TokenOffsets []int32  `parquet:"name=token_offsets, type=LIST, valuetype=INT32"`
	POSTags      []uint8  `parquet:"name=pos_tags, type=LIST, valuetype=INT32"`
	Tenses       []uint8  `parquet:"name=tenses, type=LIST, valuetype=INT32"`
	DepHashes    []uint32 `parquet:"name=dep_hashes, type=LIST, valuetype=INT32"`
}

// MinedRecord represents a single record from the Data Miner's output
type MinedRecord struct {
	FileName string `json:"file_name"`
	ChunkID  int    `json:"chunk_id"`
	Content  string `json:"content"`

	// NLP Metadata
	Tokens       []string `json:"tokens"`
	TokenOffsets []int32  `json:"token_offsets"`
	POSTags      []uint8  `json:"pos_tags"`
	Tenses       []uint8  `json:"tenses"`
	DepHashes    []uint32 `json:"dep_hashes"`
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
