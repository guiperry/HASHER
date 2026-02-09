package neural

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// HashNetwork represents a complete hash-based neural network
// as specified in HASHER_SDD.md section 4.1.1
type HashNetwork struct {
	InputSize  int           `json:"input_size"`
	Hidden1    int           `json:"hidden1"`
	Hidden2    int           `json:"hidden2"`
	OutputSize int           `json:"output_size"`
	Seeds1     [][32]byte    `json:"-"` // Hidden layer 1 seeds
	Seeds2     [][32]byte    `json:"-"` // Hidden layer 2 seeds
	SeedsOut   [][32]byte    `json:"-"` // Output layer seeds
	Neurons1   []*HashNeuron `json:"-"` // Hidden layer 1 neurons
	Neurons2   []*HashNeuron `json:"-"` // Hidden layer 2 neurons
	NeuronsOut []*HashNeuron `json:"-"` // Output layer neurons
}

// NewHashNetwork creates a new hash-based neural network with random seed initialization
func NewHashNetwork(inputSize, hidden1, hidden2, outputSize int) (*HashNetwork, error) {
	if inputSize <= 0 || hidden1 <= 0 || hidden2 <= 0 || outputSize <= 0 {
		return nil, fmt.Errorf("all network dimensions must be positive")
	}

	rand.Seed(time.Now().UnixNano())

	// Initialize seeds
	seeds1 := make([][32]byte, hidden1)
	seeds2 := make([][32]byte, hidden2)
	seedsOut := make([][32]byte, outputSize)

	for i := range seeds1 {
		rand.Read(seeds1[i][:])
	}
	for i := range seeds2 {
		rand.Read(seeds2[i][:])
	}
	for i := range seedsOut {
		rand.Read(seedsOut[i][:])
	}

	// Create neurons
	neurons1 := make([]*HashNeuron, hidden1)
	for i := range neurons1 {
		neurons1[i] = NewHashNeuron(seeds1[i], "float")
	}

	neurons2 := make([]*HashNeuron, hidden2)
	for i := range neurons2 {
		neurons2[i] = NewHashNeuron(seeds2[i], "float")
	}

	neuronsOut := make([]*HashNeuron, outputSize)
	for i := range neuronsOut {
		neuronsOut[i] = NewHashNeuron(seedsOut[i], "float")
	}

	return &HashNetwork{
		InputSize:  inputSize,
		Hidden1:    hidden1,
		Hidden2:    hidden2,
		OutputSize: outputSize,
		Seeds1:     seeds1,
		Seeds2:     seeds2,
		SeedsOut:   seedsOut,
		Neurons1:   neurons1,
		Neurons2:   neurons2,
		NeuronsOut: neuronsOut,
	}, nil
}

// Forward pass through the entire network
func (net *HashNetwork) Forward(input []byte) ([]float64, error) {
	// Hidden layer 1
	layer1Output := make([]float64, net.Hidden1)
	for i, neuron := range net.Neurons1 {
		layer1Output[i] = neuron.Forward(input)
	}

	// Hidden layer 2 (convert layer1Output to []byte)
	layer2Input := floatSliceToBytes(layer1Output)
	layer2Output := make([]float64, net.Hidden2)
	for i, neuron := range net.Neurons2 {
		layer2Output[i] = neuron.Forward(layer2Input)
	}

	// Output layer (convert layer2Output to []byte)
	outputInput := floatSliceToBytes(layer2Output)
	output := make([]float64, net.OutputSize)
	for i, neuron := range net.NeuronsOut {
		output[i] = neuron.Forward(outputInput)
	}

	return output, nil
}

// Predict returns the argmax of the network outputs
func (net *HashNetwork) Predict(input []byte) (int, float64, error) {
	output, err := net.Forward(input)
	if err != nil {
		return -1, 0.0, err
	}

	maxVal := output[0]
	maxIndex := 0
	for i, val := range output[1:] {
		if val > maxVal {
			maxVal = val
			maxIndex = i + 1
		}
	}

	return maxIndex, maxVal, nil
}

// Serialize network configuration to JSON
func (net *HashNetwork) Serialize() ([]byte, error) {
	type SerializableNetwork struct {
		InputSize  int        `json:"input_size"`
		Hidden1    int        `json:"hidden1"`
		Hidden2    int        `json:"hidden2"`
		OutputSize int        `json:"output_size"`
		Seeds1     [][32]byte `json:"seeds1"`
		Seeds2     [][32]byte `json:"seeds2"`
		SeedsOut   [][32]byte `json:"seeds_out"`
	}

	serializable := SerializableNetwork{
		InputSize:  net.InputSize,
		Hidden1:    net.Hidden1,
		Hidden2:    net.Hidden2,
		OutputSize: net.OutputSize,
		Seeds1:     net.Seeds1,
		Seeds2:     net.Seeds2,
		SeedsOut:   net.SeedsOut,
	}

	return json.Marshal(serializable)
}

// Deserialize network from JSON
func DeserializeNetwork(data []byte) (*HashNetwork, error) {
	type SerializableNetwork struct {
		InputSize  int        `json:"input_size"`
		Hidden1    int        `json:"hidden1"`
		Hidden2    int        `json:"hidden2"`
		OutputSize int        `json:"output_size"`
		Seeds1     [][32]byte `json:"seeds1"`
		Seeds2     [][32]byte `json:"seeds2"`
		SeedsOut   [][32]byte `json:"seeds_out"`
	}

	var serializable SerializableNetwork
	if err := json.Unmarshal(data, &serializable); err != nil {
		return nil, err
	}

	net := &HashNetwork{
		InputSize:  serializable.InputSize,
		Hidden1:    serializable.Hidden1,
		Hidden2:    serializable.Hidden2,
		OutputSize: serializable.OutputSize,
		Seeds1:     serializable.Seeds1,
		Seeds2:     serializable.Seeds2,
		SeedsOut:   serializable.SeedsOut,
		Neurons1:   make([]*HashNeuron, serializable.Hidden1),
		Neurons2:   make([]*HashNeuron, serializable.Hidden2),
		NeuronsOut: make([]*HashNeuron, serializable.OutputSize),
	}

	for i := range net.Neurons1 {
		net.Neurons1[i] = NewHashNeuron(net.Seeds1[i], "float")
	}
	for i := range net.Neurons2 {
		net.Neurons2[i] = NewHashNeuron(net.Seeds2[i], "float")
	}
	for i := range net.NeuronsOut {
		net.NeuronsOut[i] = NewHashNeuron(net.SeedsOut[i], "float")
	}

	return net, nil
}

// floatSliceToBytes converts a slice of float64 to a byte slice for hashing
func floatSliceToBytes(floats []float64) []byte {
	bytes := make([]byte, 0, len(floats)*8)
	for _, f := range floats {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, float64ToUint64(f))
		bytes = append(bytes, buf...)
	}
	return bytes
}

// float64ToUint64 converts float64 to uint64 for hashing purposes
func float64ToUint64(f float64) uint64 {
	// Normalize to [0, 1] range first
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return uint64(f * float64(1<<64-1))
}
