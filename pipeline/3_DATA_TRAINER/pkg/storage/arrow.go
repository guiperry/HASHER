package storage

import (
	"os"
	"path/filepath"

	"github.com/apache/arrow/go/arrow"
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/lab/hasher/data-trainer/pkg/training"
)

// GetTrainingRecordArrowSchema returns the Arrow schema for TrainingRecord
// This matches the schema used by the Data Encoder (2_DATA_ENCODER)
func GetTrainingRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "source_file", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "chunk_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "window_start", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "window_end", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "context_length", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_0", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_1", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_2", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_3", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_4", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_5", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_6", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_7", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_8", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_9", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_10", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "asic_slot_11", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "target_token_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "best_seed", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
}

// ReadTrainingRecordsFromArrowIPC reads training records from an Arrow IPC stream file
func ReadTrainingRecordsFromArrowIPC(filePath string) ([]*training.TrainingRecord, error) {
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

	var records []*training.TrainingRecord

	// Read all batches
	for r.Next() {
		batch := r.Record()
		trainingRecords, err := arrowBatchToTrainingRecords(batch)
		if err != nil {
			return nil, err
		}
		records = append(records, trainingRecords...)
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// arrowBatchToTrainingRecords converts Arrow Record to TrainingRecords
func arrowBatchToTrainingRecords(batch array.Record) ([]*training.TrainingRecord, error) {
	var records []*training.TrainingRecord

	// Get columns - matches the schema from Data Encoder
	chunkIDCol := batch.Column(1).(*array.Int32)
	asicSlot0Col := batch.Column(5).(*array.Int32)
	asicSlot1Col := batch.Column(6).(*array.Int32)
	asicSlot2Col := batch.Column(7).(*array.Int32)
	asicSlot3Col := batch.Column(8).(*array.Int32)
	asicSlot4Col := batch.Column(9).(*array.Int32)
	asicSlot5Col := batch.Column(10).(*array.Int32)
	asicSlot6Col := batch.Column(11).(*array.Int32)
	asicSlot7Col := batch.Column(12).(*array.Int32)
	asicSlot8Col := batch.Column(13).(*array.Int32)
	asicSlot9Col := batch.Column(14).(*array.Int32)
	asicSlot10Col := batch.Column(15).(*array.Int32)
	asicSlot11Col := batch.Column(16).(*array.Int32)
	targetTokenCol := batch.Column(17).(*array.Int32)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		// Map ASIC slots to FeatureVector
		var featureVector [12]uint32
		featureVector[0] = uint32(asicSlot0Col.Value(i))
		featureVector[1] = uint32(asicSlot1Col.Value(i))
		featureVector[2] = uint32(asicSlot2Col.Value(i))
		featureVector[3] = uint32(asicSlot3Col.Value(i))
		featureVector[4] = uint32(asicSlot4Col.Value(i))
		featureVector[5] = uint32(asicSlot5Col.Value(i))
		featureVector[6] = uint32(asicSlot6Col.Value(i))
		featureVector[7] = uint32(asicSlot7Col.Value(i))
		featureVector[8] = uint32(asicSlot8Col.Value(i))
		featureVector[9] = uint32(asicSlot9Col.Value(i))
		featureVector[10] = uint32(asicSlot10Col.Value(i))
		featureVector[11] = uint32(asicSlot11Col.Value(i))

		// Read other fields
		targetToken := targetTokenCol.Value(i)
		contextHash := uint32(chunkIDCol.Value(i)) // Using ChunkID as context identifier

		records = append(records, &training.TrainingRecord{
			TokenSequence: []int32{targetToken}, // Simple token sequence for now
			FeatureVector: featureVector,
			TargetToken:   targetToken,
			ContextHash:   contextHash,
		})
	}

	return records, nil
}

// WriteTrainingRecordsToArrowIPC writes training records to an Arrow IPC stream file
func WriteTrainingRecordsToArrowIPC(filePath string, records []*training.TrainingRecord) error {
	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create Arrow writer
	schema := GetTrainingRecordArrowSchema()
	w := ipc.NewWriter(file, ipc.WithSchema(schema))
	defer w.Close()

	// Convert records to Arrow record
	batch, err := trainingRecordsToArrowBatch(records, memory.NewGoAllocator())
	if err != nil {
		return err
	}

	if err := w.Write(batch); err != nil {
		return err
	}

	return nil
}

// trainingRecordsToArrowBatch converts TrainingRecords to Arrow Record
func trainingRecordsToArrowBatch(records []*training.TrainingRecord, mem memory.Allocator) (array.Record, error) {
	schema := GetTrainingRecordArrowSchema()

	// Create builders for each field
	tokenSequenceBuilder := array.NewListBuilder(mem, arrow.PrimitiveTypes.Int32)
	defer tokenSequenceBuilder.Release()

	featureVectorBuilder := array.NewFixedSizeListBuilder(mem, 12, arrow.PrimitiveTypes.Uint32)
	defer featureVectorBuilder.Release()

	targetTokenBuilder := array.NewInt32Builder(mem)
	defer targetTokenBuilder.Release()

	contextHashBuilder := array.NewUint32Builder(mem)
	defer contextHashBuilder.Release()

	// Build arrays
	for _, record := range records {
		// Build token sequence
		if record.TokenSequence != nil {
			tokenSequenceBuilder.Append(true)
			tb := tokenSequenceBuilder.ValueBuilder().(*array.Int32Builder)
			tb.AppendValues(record.TokenSequence, nil)
		} else {
			tokenSequenceBuilder.AppendNull()
		}

		// Build feature vector
		featureVectorBuilder.Append(true)
		fvb := featureVectorBuilder.ValueBuilder().(*array.Uint32Builder)
		for _, val := range record.FeatureVector {
			fvb.Append(val)
		}

		// Build other fields
		targetTokenBuilder.Append(record.TargetToken)
		if record.ContextHash != 0 {
			contextHashBuilder.Append(record.ContextHash)
		} else {
			contextHashBuilder.AppendNull()
		}
	}

	// Build arrays
	tokenSequenceArr := tokenSequenceBuilder.NewArray()
	defer tokenSequenceArr.Release()

	featureVectorArr := featureVectorBuilder.NewArray()
	defer featureVectorArr.Release()

	targetTokenArr := targetTokenBuilder.NewArray()
	defer targetTokenArr.Release()

	contextHashArr := contextHashBuilder.NewArray()
	defer contextHashArr.Release()

	// Create record - use type assertion to []Interface
	var cols []array.Interface
	cols = append(cols, tokenSequenceArr, featureVectorArr, targetTokenArr, contextHashArr)

	return array.NewRecord(schema, cols, int64(len(records))), nil
}

// readArrowFile reads training records from an Arrow IPC stream file
func (di *DataIngestor) readArrowFile(filePath string) ([]*training.TrainingRecord, error) {
	di.logger.Debug("Reading Arrow IPC stream file: %s", filepath.Base(filePath))
	return ReadTrainingRecordsFromArrowIPC(filePath)
}
