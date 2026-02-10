package storage

import (
	"os"
	"path/filepath"

	"github.com/apache/arrow/go/arrow"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/lab/hasher/data-trainer/pkg/training"
)

// GetTrainingRecordArrowSchema returns the Arrow schema for TrainingRecord
func GetTrainingRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "token_sequence", Type: arrow.ListOf(arrow.PrimitiveTypes.Int32), Nullable: true},
		{Name: "feature_vector", Type: arrow.FixedSizeListOf(12, arrow.PrimitiveTypes.Uint32), Nullable: false},
		{Name: "target_token", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "context_hash", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
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

	// Get columns
	tokenSequenceCol := batch.Column(0).(*array.List)
	featureVectorCol := batch.Column(1).(*array.FixedSizeList)
	targetTokenCol := batch.Column(2).(*array.Int32)
	contextHashCol := batch.Column(3).(*array.Uint32)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		// Read token sequence
		var tokenSequence []int32
		if !tokenSequenceCol.IsNull(i) {
			tokenSequenceOffset := tokenSequenceCol.Offsets()[i]
			tokenSequenceLength := tokenSequenceCol.Offsets()[i+1] - tokenSequenceOffset
			tokenSequenceValues := tokenSequenceCol.ListValues().(*array.Int32).Int32Values()
			tokenSequenceSlice := tokenSequenceValues[tokenSequenceOffset:tokenSequenceOffset+tokenSequenceLength]
			tokenSequence = make([]int32, len(tokenSequenceSlice))
			copy(tokenSequence, tokenSequenceSlice)
		}

		// Read feature vector
		var featureVector [12]uint32
		fvValues := featureVectorCol.ListValues().(*array.Uint32)
		baseIndex := i * 12
		for j := 0; j < 12; j++ {
			featureVector[j] = fvValues.Value(baseIndex + j)
		}

		// Read other fields
		targetToken := targetTokenCol.Value(i)
		var contextHash uint32
		if !contextHashCol.IsNull(i) {
			contextHash = contextHashCol.Value(i)
		}

		records = append(records, &training.TrainingRecord{
			TokenSequence: tokenSequence,
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
