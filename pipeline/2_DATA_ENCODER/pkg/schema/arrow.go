package schema

import (
	"os"

	"github.com/apache/arrow/go/arrow"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/memory"
)

// GetMinedRecordArrowSchema returns the Arrow schema for MinedRecord
func GetMinedRecordArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "file_name", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "chunk_id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
}

// GetTrainingFrameArrowSchema returns the Arrow schema for TrainingFrame
func GetTrainingFrameArrowSchema() *arrow.Schema {
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
		{Name: "best_seed", Type: arrow.BinaryTypes.Binary, Nullable: true},
	}, nil)
}

// WriteMinedRecordsToArrowIPC writes a batch of MinedRecords to an Arrow IPC stream file
func WriteMinedRecordsToArrowIPC(filePath string, records []MinedRecord) error {
	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create Arrow writer
	schema := GetMinedRecordArrowSchema()
	w := ipc.NewWriter(file, ipc.WithSchema(schema))
	defer w.Close()

	// Convert records to Arrow record
	batch, err := minedRecordsToArrowBatch(records, memory.NewGoAllocator())
	if err != nil {
		return err
	}

	if err := w.Write(batch); err != nil {
		return err
	}

	return nil
}

// minedRecordsToArrowBatch converts MinedRecords to Arrow Record
func minedRecordsToArrowBatch(records []MinedRecord, mem memory.Allocator) (array.Record, error) {
	schema := GetMinedRecordArrowSchema()

	// Create builders for each field
	fileNameBuilder := array.NewStringBuilder(mem)
	defer fileNameBuilder.Release()

	chunkIDBuilder := array.NewInt32Builder(mem)
	defer chunkIDBuilder.Release()

	contentBuilder := array.NewStringBuilder(mem)
	defer contentBuilder.Release()

	// Build arrays
	for _, record := range records {
		fileNameBuilder.Append(record.FileName)
		chunkIDBuilder.Append(int32(record.ChunkID))
		contentBuilder.Append(record.Content)
	}

	// Build arrays
	fileNameArr := fileNameBuilder.NewArray()
	defer fileNameArr.Release()

	chunkIDArr := chunkIDBuilder.NewArray()
	defer chunkIDArr.Release()

	contentArr := contentBuilder.NewArray()
	defer contentArr.Release()

	// Create record - use type assertion to []Interface
	var cols []array.Interface
	cols = append(cols, fileNameArr, chunkIDArr, contentArr)
	
	return array.NewRecord(schema, cols, int64(len(records))), nil
}

// ReadMinedRecordsFromArrowIPC reads MinedRecords from an Arrow IPC stream file
func ReadMinedRecordsFromArrowIPC(filePath string) ([]MinedRecord, error) {
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

	var records []MinedRecord

	// Read all batches
	for r.Next() {
		batch := r.Record()
		docs, err := arrowBatchToMinedRecords(batch)
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

// arrowBatchToMinedRecords converts Arrow Record to MinedRecords
func arrowBatchToMinedRecords(batch array.Record) ([]MinedRecord, error) {
	var records []MinedRecord

	// Get columns
	fileNameCol := batch.Column(0).(*array.String)
	chunkIDCol := batch.Column(1).(*array.Int32)
	contentCol := batch.Column(2).(*array.String)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		records = append(records, MinedRecord{
			FileName: fileNameCol.Value(i),
			ChunkID:  int(chunkIDCol.Value(i)),
			Content:  contentCol.Value(i),
		})
	}

	return records, nil
}

// WriteTrainingFramesToArrowIPC writes a batch of TrainingFrames to an Arrow IPC stream file
func WriteTrainingFramesToArrowIPC(filePath string, frames []TrainingFrame) error {
	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create Arrow writer
	schema := GetTrainingFrameArrowSchema()
	w := ipc.NewWriter(file, ipc.WithSchema(schema))
	defer w.Close()

	// Convert frames to Arrow record
	batch, err := trainingFramesToArrowBatch(frames, memory.NewGoAllocator())
	if err != nil {
		return err
	}

	if err := w.Write(batch); err != nil {
		return err
	}

	return nil
}

// trainingFramesToArrowBatch converts TrainingFrames to Arrow Record
func trainingFramesToArrowBatch(frames []TrainingFrame, mem memory.Allocator) (array.Record, error) {
	schema := GetTrainingFrameArrowSchema()

	// Create builders for each field
	sourceFileBuilder := array.NewStringBuilder(mem)
	defer sourceFileBuilder.Release()

	chunkIDBuilder := array.NewInt32Builder(mem)
	defer chunkIDBuilder.Release()

	windowStartBuilder := array.NewInt32Builder(mem)
	defer windowStartBuilder.Release()

	windowEndBuilder := array.NewInt32Builder(mem)
	defer windowEndBuilder.Release()

	contextLengthBuilder := array.NewInt32Builder(mem)
	defer contextLengthBuilder.Release()

	asicSlot0Builder := array.NewInt32Builder(mem)
	defer asicSlot0Builder.Release()

	asicSlot1Builder := array.NewInt32Builder(mem)
	defer asicSlot1Builder.Release()

	asicSlot2Builder := array.NewInt32Builder(mem)
	defer asicSlot2Builder.Release()

	asicSlot3Builder := array.NewInt32Builder(mem)
	defer asicSlot3Builder.Release()

	asicSlot4Builder := array.NewInt32Builder(mem)
	defer asicSlot4Builder.Release()

	asicSlot5Builder := array.NewInt32Builder(mem)
	defer asicSlot5Builder.Release()

	asicSlot6Builder := array.NewInt32Builder(mem)
	defer asicSlot6Builder.Release()

	asicSlot7Builder := array.NewInt32Builder(mem)
	defer asicSlot7Builder.Release()

	asicSlot8Builder := array.NewInt32Builder(mem)
	defer asicSlot8Builder.Release()

	asicSlot9Builder := array.NewInt32Builder(mem)
	defer asicSlot9Builder.Release()

	asicSlot10Builder := array.NewInt32Builder(mem)
	defer asicSlot10Builder.Release()

	asicSlot11Builder := array.NewInt32Builder(mem)
	defer asicSlot11Builder.Release()

	targetTokenIDBuilder := array.NewInt32Builder(mem)
	defer targetTokenIDBuilder.Release()

	bestSeedBuilder := array.NewBinaryBuilder(mem, arrow.BinaryTypes.Binary)
	defer bestSeedBuilder.Release()

	// Build arrays
	for _, frame := range frames {
		sourceFileBuilder.Append(frame.SourceFile)
		chunkIDBuilder.Append(frame.ChunkID)
		windowStartBuilder.Append(frame.WindowStart)
		windowEndBuilder.Append(frame.WindowEnd)
		contextLengthBuilder.Append(frame.ContextLength)
		asicSlot0Builder.Append(frame.AsicSlots0)
		asicSlot1Builder.Append(frame.AsicSlots1)
		asicSlot2Builder.Append(frame.AsicSlots2)
		asicSlot3Builder.Append(frame.AsicSlots3)
		asicSlot4Builder.Append(frame.AsicSlots4)
		asicSlot5Builder.Append(frame.AsicSlots5)
		asicSlot6Builder.Append(frame.AsicSlots6)
		asicSlot7Builder.Append(frame.AsicSlots7)
		asicSlot8Builder.Append(frame.AsicSlots8)
		asicSlot9Builder.Append(frame.AsicSlots9)
		asicSlot10Builder.Append(frame.AsicSlots10)
		asicSlot11Builder.Append(frame.AsicSlots11)
		targetTokenIDBuilder.Append(frame.TargetTokenID)
		if frame.BestSeed != nil {
			bestSeedBuilder.Append(frame.BestSeed)
		} else {
			bestSeedBuilder.AppendNull()
		}
	}

	// Build arrays
	sourceFileArr := sourceFileBuilder.NewArray()
	defer sourceFileArr.Release()

	chunkIDArr := chunkIDBuilder.NewArray()
	defer chunkIDArr.Release()

	windowStartArr := windowStartBuilder.NewArray()
	defer windowStartArr.Release()

	windowEndArr := windowEndBuilder.NewArray()
	defer windowEndArr.Release()

	contextLengthArr := contextLengthBuilder.NewArray()
	defer contextLengthArr.Release()

	asicSlot0Arr := asicSlot0Builder.NewArray()
	defer asicSlot0Arr.Release()

	asicSlot1Arr := asicSlot1Builder.NewArray()
	defer asicSlot1Arr.Release()

	asicSlot2Arr := asicSlot2Builder.NewArray()
	defer asicSlot2Arr.Release()

	asicSlot3Arr := asicSlot3Builder.NewArray()
	defer asicSlot3Arr.Release()

	asicSlot4Arr := asicSlot4Builder.NewArray()
	defer asicSlot4Arr.Release()

	asicSlot5Arr := asicSlot5Builder.NewArray()
	defer asicSlot5Arr.Release()

	asicSlot6Arr := asicSlot6Builder.NewArray()
	defer asicSlot6Arr.Release()

	asicSlot7Arr := asicSlot7Builder.NewArray()
	defer asicSlot7Arr.Release()

	asicSlot8Arr := asicSlot8Builder.NewArray()
	defer asicSlot8Arr.Release()

	asicSlot9Arr := asicSlot9Builder.NewArray()
	defer asicSlot9Arr.Release()

	asicSlot10Arr := asicSlot10Builder.NewArray()
	defer asicSlot10Arr.Release()

	asicSlot11Arr := asicSlot11Builder.NewArray()
	defer asicSlot11Arr.Release()

	targetTokenIDArr := targetTokenIDBuilder.NewArray()
	defer targetTokenIDArr.Release()

	bestSeedArr := bestSeedBuilder.NewArray()
	defer bestSeedArr.Release()

	// Create record - use type assertion to []Interface
	var cols []array.Interface
	cols = append(cols,
		sourceFileArr, chunkIDArr, windowStartArr, windowEndArr, contextLengthArr,
		asicSlot0Arr, asicSlot1Arr, asicSlot2Arr, asicSlot3Arr,
		asicSlot4Arr, asicSlot5Arr, asicSlot6Arr, asicSlot7Arr,
		asicSlot8Arr, asicSlot9Arr, asicSlot10Arr, asicSlot11Arr,
		targetTokenIDArr, bestSeedArr,
	)
	
	return array.NewRecord(schema, cols, int64(len(frames))), nil
}

// ReadTrainingFramesFromArrowIPC reads TrainingFrames from an Arrow IPC stream file
func ReadTrainingFramesFromArrowIPC(filePath string) ([]TrainingFrame, error) {
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

	var frames []TrainingFrame

	// Read all batches
	for r.Next() {
		batch := r.Record()
		trainingFrames, err := arrowBatchToTrainingFrames(batch)
		if err != nil {
			return nil, err
		}
		frames = append(frames, trainingFrames...)
	}

	if err := r.Err(); err != nil {
		return nil, err
	}

	return frames, nil
}

// arrowBatchToTrainingFrames converts Arrow Record to TrainingFrames
func arrowBatchToTrainingFrames(batch array.Record) ([]TrainingFrame, error) {
	var frames []TrainingFrame

	// Get columns
	sourceFileCol := batch.Column(0).(*array.String)
	chunkIDCol := batch.Column(1).(*array.Int32)
	windowStartCol := batch.Column(2).(*array.Int32)
	windowEndCol := batch.Column(3).(*array.Int32)
	contextLengthCol := batch.Column(4).(*array.Int32)
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
	targetTokenIDCol := batch.Column(17).(*array.Int32)
	bestSeedCol := batch.Column(18).(*array.Binary)

	// Convert each row
	for i := 0; i < int(batch.NumRows()); i++ {
		var seed []byte
		if !bestSeedCol.IsNull(i) {
			seed = bestSeedCol.Value(i)
		}
		frames = append(frames, TrainingFrame{
			SourceFile:     sourceFileCol.Value(i),
			ChunkID:        chunkIDCol.Value(i),
			WindowStart:    windowStartCol.Value(i),
			WindowEnd:      windowEndCol.Value(i),
			ContextLength:  contextLengthCol.Value(i),
			AsicSlots0:     asicSlot0Col.Value(i),
			AsicSlots1:     asicSlot1Col.Value(i),
			AsicSlots2:     asicSlot2Col.Value(i),
			AsicSlots3:     asicSlot3Col.Value(i),
			AsicSlots4:     asicSlot4Col.Value(i),
			AsicSlots5:     asicSlot5Col.Value(i),
			AsicSlots6:     asicSlot6Col.Value(i),
			AsicSlots7:     asicSlot7Col.Value(i),
			AsicSlots8:     asicSlot8Col.Value(i),
			AsicSlots9:     asicSlot9Col.Value(i),
			AsicSlots10:    asicSlot10Col.Value(i),
			AsicSlots11:    asicSlot11Col.Value(i),
			TargetTokenID:  targetTokenIDCol.Value(i),
			BestSeed:       seed,
		})
	}

	return frames, nil
}
