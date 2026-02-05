package hasher

import (
	"math"
)

// MatrixSeedEncoder handles conversion between weight matrices and cryptographic seeds
type MatrixSeedEncoder struct {
	UseFactorization bool
	Precision        string // "fp16", "fp32"
	Compression      bool
}

// NewMatrixSeedEncoder creates a new seed encoder
func NewMatrixSeedEncoder() *MatrixSeedEncoder {
	return &MatrixSeedEncoder{
		UseFactorization: true,   // Default to factorized for space efficiency
		Precision:        "fp16", // 16-bit fixed point
		Compression:      true,   // Enable compression
	}
}

// EncodeMatrix encodes a weight matrix and bias into a 32-byte seed
func (e *MatrixSeedEncoder) EncodeMatrix(weights [][]float32, bias []float32) [32]byte {
	if e.UseFactorization {
		return e.encodeFactorized(weights, bias)
	}
	return e.encodeDense(weights, bias)
}

// encodeFactorized uses low-rank factorization to fit larger matrices
func (e *MatrixSeedEncoder) encodeFactorized(weights [][]float32, bias []float32) [32]byte {
	// Perform simple SVD-like factorization: W ≈ U·V^T
	// For simplicity, use rank-4 factorization for any matrix size
	rank := 4
	rows := len(weights)
	cols := len(weights[0])

	// Random initialization of factors (in real implementation, use proper SVD)
	U := make([][]float32, rows)
	V := make([][]float32, cols)

	for i := 0; i < rows; i++ {
		U[i] = make([]float32, rank)
		for j := 0; j < rank; j++ {
			U[i][j] = weights[i][0] // Simplified: use first column
		}
	}

	for i := 0; i < cols; i++ {
		V[i] = make([]float32, rank)
		for j := 0; j < rank; j++ {
			V[i][j] = weights[0][i] // Simplified: use first row
		}
	}

	// Pack factorized matrices into 32 bytes
	data := make([]byte, 0, 32)

	// Pack U matrix (rows × rank)
	for i := 0; i < rows && i < 4; i++ { // Max 4 rows in 32 bytes
		for j := 0; j < rank; j++ {
			quantized := e.quantizeFP16(U[i][j])
			data = append(data, byte(quantized>>8), byte(quantized&0xFF))
		}
	}

	// Pack V matrix (cols × rank)
	for i := 0; i < cols && len(data) < 28; i++ { // Space remaining
		for j := 0; j < rank && len(data) < 30; j++ {
			quantized := e.quantizeFP16(V[i][j])
			data = append(data, byte(quantized>>8), byte(quantized&0xFF))
		}
	}

	// Pack bias (single value approximation)
	if len(bias) > 0 {
		biasVal := bias[0] // Use first bias value
		quantized := e.quantizeFP16(biasVal)
		data = append(data, byte(quantized>>8), byte(quantized&0xFF))
	}

	// Fill remaining space with metadata
	for len(data) < 32 {
		data = append(data, 0)
	}

	// Add error detection
	checksum := e.computeChecksum(data[:30])
	data[30] = byte(checksum >> 8)
	data[31] = byte(checksum & 0xFF)

	var seed [32]byte
	copy(seed[:], data)
	return seed
}

// encodeDense packs smaller matrices directly
func (e *MatrixSeedEncoder) encodeDense(weights [][]float32, bias []float32) [32]byte {
	data := make([]byte, 0, 32)

	// Limit to 4x4 matrix for 32 bytes
	maxRows := 4
	maxCols := 4

	for i := 0; i < len(weights) && i < maxRows; i++ {
		for j := 0; j < len(weights[i]) && j < maxCols; j++ {
			quantized := e.quantizeFP16(weights[i][j])
			data = append(data, byte(quantized>>8), byte(quantized&0xFF))
		}
	}

	// Pack bias values
	for i := 0; i < len(bias) && len(data) < 30; i++ {
		quantized := e.quantizeFP16(bias[i])
		data = append(data, byte(quantized>>8), byte(quantized&0xFF))
	}

	// Pad to 32 bytes
	for len(data) < 32 {
		data = append(data, 0)
	}

	var seed [32]byte
	copy(seed[:], data)
	return seed
}

// DecodeMatrix decodes a 32-byte seed back to weight matrix and bias
func (e *MatrixSeedEncoder) DecodeMatrix(seed [32]byte, rows, cols int) ([][]float32, []float32) {
	if e.UseFactorization {
		return e.decodeFactorized(seed, rows, cols)
	}
	return e.decodeDense(seed, rows, cols)
}

// decodeFactorized reconstructs matrix from factorized representation
func (e *MatrixSeedEncoder) decodeFactorized(seed [32]byte, rows, cols int) ([][]float32, []float32) {
	rank := 4

	// Extract U factor
	U := make([][]float32, rows)
	for i := 0; i < rows && i < 4; i++ {
		U[i] = make([]float32, rank)
		for j := 0; j < rank; j++ {
			offset := (i*rank + j) * 2
			if offset+1 < 30 {
				fp16 := uint16(seed[offset])<<8 | uint16(seed[offset+1])
				U[i][j] = e.dequantizeFP16(fp16)
			}
		}
	}

	// Extract V factor
	V := make([][]float32, cols)
	for i := 0; i < cols && i < 4; i++ {
		V[i] = make([]float32, rank)
		for j := 0; j < rank; j++ {
			offset := 32 + (i*rank+j)*2 // Simplified offset
			if offset+1 < 30 {
				fp16 := uint16(seed[offset])<<8 | uint16(seed[offset+1])
				V[i][j] = e.dequantizeFP16(fp16)
			}
		}
	}

	// Reconstruct matrix: W ≈ U·V^T
	weights := make([][]float32, rows)
	for i := 0; i < rows; i++ {
		weights[i] = make([]float32, cols)
		for j := 0; j < cols; j++ {
			sum := float32(0)
			for k := 0; k < rank; k++ {
				if i < len(U) && j < len(V) {
					sum += U[i][k] * V[j][k] // Note: V transposed
				}
			}
			weights[i][j] = sum
		}
	}

	// Extract bias
	bias := make([]float32, cols)
	offset := 28 // Bias starts at byte 28
	for i := 0; i < cols && offset+1 < 32; i++ {
		fp16 := uint16(seed[offset])<<8 | uint16(seed[offset+1])
		bias[i] = e.dequantizeFP16(fp16)
		offset += 2
	}

	return weights, bias
}

// decodeDense extracts dense matrix from seed
func (e *MatrixSeedEncoder) decodeDense(seed [32]byte, rows, cols int) ([][]float32, []float32) {
	weights := make([][]float32, rows)
	for i := 0; i < rows && i < 4; i++ {
		weights[i] = make([]float32, cols)
		for j := 0; j < cols && j < 4; j++ {
			offset := (i*4 + j) * 2
			fp16 := uint16(seed[offset])<<8 | uint16(seed[offset+1])
			weights[i][j] = e.dequantizeFP16(fp16)
		}
	}

	// Extract bias
	bias := make([]float32, cols)
	offset := 24 // Start after 4x4 matrix (16*2 = 32 bytes, but we overlap)
	for i := 0; i < cols && offset+1 < 32; i++ {
		fp16 := uint16(seed[offset])<<8 | uint16(seed[offset+1])
		bias[i] = e.dequantizeFP16(fp16)
		offset += 2
	}

	return weights, bias
}

// quantizeFP16 converts float32 to 16-bit fixed point
func (e *MatrixSeedEncoder) quantizeFP16(value float32) uint16 {
	// Simple fixed-point: sign + 7-bit exponent + 8-bit mantissa
	// For simplicity, use linear scaling to [-1, 1] range
	clamped := math.Max(-1.0, math.Min(1.0, float64(value)))
	quantized := uint16((clamped + 1.0) * 32767.5)
	return quantized
}

// dequantizeFP16 converts 16-bit fixed point to float32
func (e *MatrixSeedEncoder) dequantizeFP16(value uint16) float32 {
	return float32(value)/32767.5 - 1.0
}

// computeChecksum calculates simple checksum for error detection
func (e *MatrixSeedEncoder) computeChecksum(data []byte) uint16 {
	var sum uint16
	for _, b := range data {
		sum += uint16(b)
	}
	return sum
}

// ValidateSeed checks if seed data is valid using checksum
func (e *MatrixSeedEncoder) ValidateSeed(seed [32]byte) bool {
	checksum := uint16(seed[30])<<8 | uint16(seed[31])
	computed := e.computeChecksum(seed[:30])
	return checksum == computed
}
