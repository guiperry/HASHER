package app

import (
	"os"

	"github.com/apache/arrow/go/arrow"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/memory"
)

// GetDocumentRecordArrowSchema returns the Arrow schema for DocumentRecord
func GetDocumentRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "file_name", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "chunk_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "embedding", Type: arrow.ListOf(arrow.PrimitiveTypes.Float32), Nullable: false},
	}, nil)
}

// WriteDocumentRecordsToArrowIPC writes a batch of DocumentRecords to an Arrow IPC stream file
func WriteDocumentRecordsToArrowIPC(filePath string, records []DocumentRecord) error {
	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create Arrow writer
	schema := GetDocumentRecordArrowSchema()
	w := ipc.NewWriter(file, ipc.WithSchema(schema))
	defer w.Close()

	// Convert records to Arrow record
	batch, err := documentRecordsToArrowBatch(records, memory.NewGoAllocator())
	if err != nil {
		return err
	}

	if err := w.Write(batch); err != nil {
		return err
	}

	return nil
}

// documentRecordsToArrowBatch converts DocumentRecords to Arrow Record
func documentRecordsToArrowBatch(records []DocumentRecord, mem memory.Allocator) (array.Record, error) {
	schema := GetDocumentRecordArrowSchema()

	// Create builders for each field
	fileNameBuilder := array.NewStringBuilder(mem)
	defer fileNameBuilder.Release()

	chunkIDBuilder := array.NewInt32Builder(mem)
	defer chunkIDBuilder.Release()

	contentBuilder := array.NewStringBuilder(mem)
	defer contentBuilder.Release()

	embeddingBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Float32)
	defer embeddingBuilder.Release()

	// Build arrays
	for _, record := range records {
		fileNameBuilder.Append(record.FileName)
		chunkIDBuilder.Append(record.ChunkID)
		contentBuilder.Append(record.Content)

		// Build embedding array
		embeddingBuilder.Append(true)
		fb := embeddingBuilder.ValueBuilder().(*array.Float32Builder)
		fb.AppendValues(record.Embedding, nil)
	}

	// Build arrays
	fileNameArr := fileNameBuilder.NewArray()
	defer fileNameArr.Release()

	chunkIDArr := chunkIDBuilder.NewArray()
	defer chunkIDArr.Release()

	contentArr := contentBuilder.NewArray()
	defer contentArr.Release()

	embeddingArr := embeddingBuilder.NewArray()
	defer embeddingArr.Release()

	// Create record - use type assertion to []Interface
	var cols []array.Interface
	cols = append(cols, fileNameArr, chunkIDArr, contentArr, embeddingArr)
	
	return array.NewRecord(schema, cols, int64(len(records))), nil
}

// ReadDocumentRecordsFromArrowIPC reads DocumentRecords from an Arrow IPC stream file
func ReadDocumentRecordsFromArrowIPC(filePath string) ([]DocumentRecord, error) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create Arrow reader
	r, err := ipc.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer r.Release()

	var records []DocumentRecord

	// Read all batches
	for r.Next() {
		batch := r.Record()
		docs, err := arrowBatchToDocumentRecords(batch)
		if err != nil {
			return nil, err
		}
		records = append(records, docs...)
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// arrowBatchToDocumentRecords converts Arrow Record to DocumentRecords
func arrowBatchToDocumentRecords(batch array.Record) ([]DocumentRecord, error) {
	var records []DocumentRecord

	// Get columns
	fileNameCol := batch.Column(0).(*array.String)
	chunkIDCol := batch.Column(1).(*array.Int32)
	contentCol := batch.Column(2).(*array.String)
	embeddingCol := batch.Column(3).(*array.List)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		// Get embedding values
		embeddingOffset := embeddingCol.Offsets()[i]
		embeddingLength := embeddingCol.Offsets()[i+1] - embeddingOffset
		embeddingValues := embeddingCol.ListValues().(*array.Float32).Float32Values()
		embeddingSlice := embeddingValues[embeddingOffset:embeddingOffset+embeddingLength]

		// Create copy to avoid reference issues
		embeddingCopy := make([]float32, len(embeddingSlice))
		copy(embeddingCopy, embeddingSlice)

		records = append(records, DocumentRecord{
			FileName:  fileNameCol.Value(i),
			ChunkID:   chunkIDCol.Value(i),
			Content:   contentCol.Value(i),
			Embedding: embeddingCopy,
		})
	}

	return records, nil
}
