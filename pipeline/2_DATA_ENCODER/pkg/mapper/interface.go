package mapper

import (
	"fmt"

	"data-encoder/pkg/schema"
)

type NeuralFrame struct {
	Slots         [12]uint32
	TargetTokenID int32
	SourceRef     string
	Metadata      map[string]interface{}
}

type Mapper interface {
	Map(input string) (NeuralFrame, error)
	Schema() *SlotSchema
}

func MapperFactory(schema *SlotSchema) (Mapper, error) {
	switch schema.Domain.Name {
	case "MATHASHER":
		return NewSchemaLaTeXMapper(schema), nil
	case "HASHER":
		return NewSchemaVarianceMapper(schema), nil
	default:
		return nil, fmt.Errorf("unknown domain: %s", schema.Domain.Name)
	}
}

func NeuralFrameToSchemaFrame(frame NeuralFrame) schema.TrainingFrame {
	return schema.TrainingFrame{
		SourceFile:    frame.SourceRef,
		ChunkID:       0,
		WindowStart:   0,
		WindowEnd:     0,
		ContextLength: 0,
		AsicSlots0:    int32(frame.Slots[0]),
		AsicSlots1:    int32(frame.Slots[1]),
		AsicSlots2:    int32(frame.Slots[2]),
		AsicSlots3:    int32(frame.Slots[3]),
		AsicSlots4:    int32(frame.Slots[4]),
		AsicSlots5:    int32(frame.Slots[5]),
		AsicSlots6:    int32(frame.Slots[6]),
		AsicSlots7:    int32(frame.Slots[7]),
		AsicSlots8:    int32(frame.Slots[8]),
		AsicSlots9:    int32(frame.Slots[9]),
		AsicSlots10:   int32(frame.Slots[10]),
		AsicSlots11:   int32(frame.Slots[11]),
		TargetTokenID: frame.TargetTokenID,
		BestSeed:      nil,
	}
}
