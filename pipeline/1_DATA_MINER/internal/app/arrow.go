package app

import (
	"os"

	"github.com/apache/arrow/go/arrow"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/memory"
)

// GetAlpacaDocumentRecordArrowSchema returns the Arrow schema for AlpacaDocumentRecord
func GetAlpacaDocumentRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "instruction", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "input", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "output", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "file_name", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "chunk_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "embedding", Type: arrow.ListOf(arrow.PrimitiveTypes.Float32), Nullable: false},
		{Name: "tokens", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: false},
		{Name: "token_offsets", Type: arrow.ListOf(arrow.PrimitiveTypes.Int32), Nullable: false},
		{Name: "pos_tags", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint8), Nullable: false},
		{Name: "tenses", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint8), Nullable: false},
		{Name: "dep_hashes", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint32), Nullable: false},
	}, nil)
}

// WriteAlpacaDocumentRecordsToArrowIPC writes a batch of AlpacaDocumentRecords to an Arrow IPC stream file
func WriteAlpacaDocumentRecordsToArrowIPC(filePath string, records []AlpacaDocumentRecord) error {
	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create Arrow writer
	schema := GetAlpacaDocumentRecordArrowSchema()
	w := ipc.NewWriter(file, ipc.WithSchema(schema))
	defer w.Close()

	// Convert records to Arrow record
	batch, err := alpacaDocumentRecordsToArrowBatch(records, memory.NewGoAllocator())
	if err != nil {
		return err
	}

	if err := w.Write(batch); err != nil {
		return err
	}

	return nil
}

// alpacaDocumentRecordsToArrowBatch converts AlpacaDocumentRecords to Arrow Record
func alpacaDocumentRecordsToArrowBatch(records []AlpacaDocumentRecord, mem memory.Allocator) (array.Record, error) {
	schema := GetAlpacaDocumentRecordArrowSchema()

	// Create builders for each field
	instructionBuilder := array.NewStringBuilder(mem)
	defer instructionBuilder.Release()

	inputBuilder := array.NewStringBuilder(mem)
	defer inputBuilder.Release()

	outputBuilder := array.NewStringBuilder(mem)
	defer outputBuilder.Release()

	fileNameBuilder := array.NewStringBuilder(mem)
	defer fileNameBuilder.Release()

	chunkIDBuilder := array.NewInt32Builder(mem)
	defer chunkIDBuilder.Release()

	embeddingBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Float32)
	defer embeddingBuilder.Release()

	tokensBuilder := array.NewListBuilder(mem, arrow.BinaryTypes.String)
	defer tokensBuilder.Release()

	tokenOffsetsBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Int32)
	defer tokenOffsetsBuilder.Release()

	posTagsBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint8)
	defer posTagsBuilder.Release()

	tensesBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint8)
	defer tensesBuilder.Release()

	depHashesBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint32)
	defer depHashesBuilder.Release()

	// Build arrays
	for _, record := range records {
		instructionBuilder.Append(record.Instruction)
		inputBuilder.Append(record.Input)
		outputBuilder.Append(record.Output)
		fileNameBuilder.Append(record.FileName)
		chunkIDBuilder.Append(record.ChunkID)

		// Build embedding array
		embeddingBuilder.Append(true)
		fb := embeddingBuilder.ValueBuilder().(*array.Float32Builder)
		fb.AppendValues(record.Embedding, nil)

		// Build tokens array
		tokensBuilder.Append(true)
		tb := tokensBuilder.ValueBuilder().(*array.StringBuilder)
		tb.AppendValues(record.Tokens, nil)

		// Build token_offsets array
		tokenOffsetsBuilder.Append(true)
		tob := tokenOffsetsBuilder.ValueBuilder().(*array.Int32Builder)
		tob.AppendValues(record.TokenOffsets, nil)

		// Build pos_tags array
		posTagsBuilder.Append(true)
		pb := posTagsBuilder.ValueBuilder().(*array.Uint8Builder)
		pb.AppendValues(record.POSTags, nil)

		// Build tenses array
		tensesBuilder.Append(true)
		tensb := tensesBuilder.ValueBuilder().(*array.Uint8Builder)
		tensb.AppendValues(record.Tenses, nil)

		// Build dep_hashes array
		depHashesBuilder.Append(true)
		db := depHashesBuilder.ValueBuilder().(*array.Uint32Builder)
		db.AppendValues(record.DepHashes, nil)
	}

	// Build arrays
	instructionArr := instructionBuilder.NewArray()
	defer instructionArr.Release()

	inputArr := inputBuilder.NewArray()
	defer inputArr.Release()

	outputArr := outputBuilder.NewArray()
	defer outputArr.Release()

	fileNameArr := fileNameBuilder.NewArray()
	defer fileNameArr.Release()

	chunkIDArr := chunkIDBuilder.NewArray()
	defer chunkIDArr.Release()

	embeddingArr := embeddingBuilder.NewArray()
	defer embeddingArr.Release()

	tokensArr := tokensBuilder.NewArray()
	defer tokensArr.Release()

	posTagsArr := posTagsBuilder.NewArray()
	defer posTagsArr.Release()

	tensesArr := tensesBuilder.NewArray()
	defer tensesArr.Release()

	depHashesArr := depHashesBuilder.NewArray()
	defer depHashesArr.Release()

	tokenOffsetsArr := tokenOffsetsBuilder.NewArray()
	defer tokenOffsetsArr.Release()

	// Create record
	var cols []array.Interface
	cols = append(cols, instructionArr, inputArr, outputArr, fileNameArr, chunkIDArr, embeddingArr, tokensArr, tokenOffsetsArr, posTagsArr, tensesArr, depHashesArr)
	
	return array.NewRecord(schema, cols, int64(len(records))), nil
}

// GetDocumentRecordArrowSchema returns the Arrow schema for DocumentRecord
func GetDocumentRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "file_name", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "chunk_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "embedding", Type: arrow.ListOf(arrow.PrimitiveTypes.Float32), Nullable: false},
		{Name: "tokens", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: false},
		{Name: "token_offsets", Type: arrow.ListOf(arrow.PrimitiveTypes.Int32), Nullable: false},
		{Name: "pos_tags", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint8), Nullable: false},
		{Name: "tenses", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint8), Nullable: false},
		{Name: "dep_hashes", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint32), Nullable: false},
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

	tokensBuilder := array.NewListBuilder(mem, arrow.BinaryTypes.String)
	defer tokensBuilder.Release()

	tokenOffsetsBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Int32)
	defer tokenOffsetsBuilder.Release()

	posTagsBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint8)
	defer posTagsBuilder.Release()

	tensesBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint8)
	defer tensesBuilder.Release()

	depHashesBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Uint32)
	defer depHashesBuilder.Release()

	// Build arrays
	for _, record := range records {
		fileNameBuilder.Append(record.FileName)
		chunkIDBuilder.Append(record.ChunkID)
		contentBuilder.Append(record.Content)

		// Build embedding array
		embeddingBuilder.Append(true)
		fb := embeddingBuilder.ValueBuilder().(*array.Float32Builder)
		fb.AppendValues(record.Embedding, nil)

		// Build tokens array
		tokensBuilder.Append(true)
		tb := tokensBuilder.ValueBuilder().(*array.StringBuilder)
		tb.AppendValues(record.Tokens, nil)

		// Build token_offsets array
		tokenOffsetsBuilder.Append(true)
		tob := tokenOffsetsBuilder.ValueBuilder().(*array.Int32Builder)
		tob.AppendValues(record.TokenOffsets, nil)

		// Build pos_tags array
		posTagsBuilder.Append(true)
		pb := posTagsBuilder.ValueBuilder().(*array.Uint8Builder)
		pb.AppendValues(record.POSTags, nil)

		// Build tenses array
		tensesBuilder.Append(true)
		tensb := tensesBuilder.ValueBuilder().(*array.Uint8Builder)
		tensb.AppendValues(record.Tenses, nil)

		// Build dep_hashes array
		depHashesBuilder.Append(true)
		db := depHashesBuilder.ValueBuilder().(*array.Uint32Builder)
		db.AppendValues(record.DepHashes, nil)
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

	tokensArr := tokensBuilder.NewArray()
	defer tokensArr.Release()

	posTagsArr := posTagsBuilder.NewArray()
	defer posTagsArr.Release()

	tensesArr := tensesBuilder.NewArray()
	defer tensesArr.Release()

	depHashesArr := depHashesBuilder.NewArray()
	defer depHashesArr.Release()

	tokenOffsetsArr := tokenOffsetsBuilder.NewArray()
	defer tokenOffsetsArr.Release()

	// Create record
	var cols []array.Interface
	cols = append(cols, fileNameArr, chunkIDArr, contentArr, embeddingArr, tokensArr, tokenOffsetsArr, posTagsArr, tensesArr, depHashesArr)
	
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
	tokensCol := batch.Column(4).(*array.List)
	tokenOffsetsCol := batch.Column(5).(*array.List)
	posTagsCol := batch.Column(6).(*array.List)
	tensesCol := batch.Column(7).(*array.List)
	depHashesCol := batch.Column(8).(*array.List)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		// Get embedding values
		embeddingOffset := embeddingCol.Offsets()[i]
		embeddingLength := embeddingCol.Offsets()[i+1] - embeddingOffset
		embeddingValues := embeddingCol.ListValues().(*array.Float32).Float32Values()
		embeddingSlice := embeddingValues[embeddingOffset:embeddingOffset+embeddingLength]

		embeddingCopy := make([]float32, len(embeddingSlice))
		copy(embeddingCopy, embeddingSlice)

		// Get tokens
		tokensOffset := tokensCol.Offsets()[i]
		tokensLength := tokensCol.Offsets()[i+1] - tokensOffset
		tokensValues := tokensCol.ListValues().(*array.String)
		tokensSlice := make([]string, tokensLength)
		for j := 0; j < int(tokensLength); j++ {
			tokensSlice[j] = tokensValues.Value(int(tokensOffset) + j)
		}

		// Get token_offsets
		tokenOffsetsOffset := tokenOffsetsCol.Offsets()[i]
		tokenOffsetsLength := tokenOffsetsCol.Offsets()[i+1] - tokenOffsetsOffset
		tokenOffsetsValues := tokenOffsetsCol.ListValues().(*array.Int32).Int32Values()
		tokenOffsetsSlice := make([]int32, tokenOffsetsLength)
		copy(tokenOffsetsSlice, tokenOffsetsValues[tokenOffsetsOffset:tokenOffsetsOffset+tokenOffsetsLength])

		// Get pos_tags
		posTagsOffset := posTagsCol.Offsets()[i]
		posTagsLength := posTagsCol.Offsets()[i+1] - posTagsOffset
		posTagsValues := posTagsCol.ListValues().(*array.Uint8).Uint8Values()
		posTagsSlice := make([]uint8, posTagsLength)
		copy(posTagsSlice, posTagsValues[posTagsOffset:posTagsOffset+posTagsLength])

		// Get tenses
		tensesOffset := tensesCol.Offsets()[i]
		tensesLength := tensesCol.Offsets()[i+1] - tensesOffset
		tensesValues := tensesCol.ListValues().(*array.Uint8).Uint8Values()
		tensesSlice := make([]uint8, tensesLength)
		copy(tensesSlice, tensesValues[tensesOffset:tensesOffset+tensesLength])

		// Get dep_hashes
		depHashesOffset := depHashesCol.Offsets()[i]
		depHashesLength := depHashesCol.Offsets()[i+1] - depHashesOffset
		depHashesValues := depHashesCol.ListValues().(*array.Uint32).Uint32Values()
		depHashesSlice := make([]uint32, depHashesLength)
		copy(depHashesSlice, depHashesValues[depHashesOffset:depHashesOffset+depHashesLength])

		records = append(records, DocumentRecord{
			FileName:     fileNameCol.Value(i),
			ChunkID:      chunkIDCol.Value(i),
			Content:      contentCol.Value(i),
			Embedding:    embeddingCopy,
			Tokens:       tokensSlice,
			TokenOffsets: tokenOffsetsSlice,
			POSTags:      posTagsSlice,
			Tenses:       tensesSlice,
			DepHashes:    depHashesSlice,
		})
	}

	return records, nil
}
