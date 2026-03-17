package api

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"

	"hasher/pkg/hashing/math"
	"hasher/pkg/hashing/miner"
)

type MathVerifierServer struct {
	watchdog  *math.InferenceWatchdog
	mapper    *math.LaTeXMapper
	subdomain uint32
}

func NewMathVerifierServer(subdomain uint32) *MathVerifierServer {
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}
	return &MathVerifierServer{
		watchdog:  math.NewInferenceWatchdog(subdomain),
		mapper:    math.NewLaTeXMapper(subdomain),
		subdomain: subdomain,
	}
}

func (s *MathVerifierServer) Verify(latex string, subdomain uint32) *MathVerifyResponse {
	start := time.Now()

	if subdomain == 0 {
		subdomain = s.subdomain
	}

	slots := s.mapper.MapLaTeXToTensor(latex, subdomain)
	validation := s.watchdog.ValidateMathStep(0, slots)
	latency := time.Since(start).Seconds() * 1000

	response := &MathVerifyResponse{
		Status:            "UNVERIFIED",
		Nonce:             fmt.Sprintf("0x%08x", slots[11]),
		ResultHash:        fmt.Sprintf("0x%08x", slots[3]),
		DetokenizedOutput: latex,
		LogicIntegrity:    float32(validation.LogicIntegrity),
		LatencyMs:         float32(latency),
	}

	if validation.Valid {
		response.Status = "VERIFIED"
	}

	if !validation.Valid && validation.Error != "" {
		response.Status = "REJECTED"
		response.ResultHash = validation.Error
	}

	return response
}

type MathVerifyResponse struct {
	Status            string
	Nonce             string
	ResultHash        string
	DetokenizedOutput string
	LogicIntegrity    float32
	LatencyMs         float32
}

type ServerConfig struct {
	Port      string
	Subdomain uint32
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:      ":50051",
		Subdomain: math.SUBDOMAIN_ARITHMETIC,
	}
}

func StartServer(config *ServerConfig) error {
	if config == nil {
		config = DefaultServerConfig()
	}

	lis, err := net.Listen("tcp", config.Port)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	server := NewMathVerifierServer(config.Subdomain)
	RegisterMathVerifierService(grpcServer, server)

	log.Printf("HashNet Math Verification API listening on %s", config.Port)

	return grpcServer.Serve(lis)
}

type MathVerificationRequest struct {
	Context             string  `json:"context"`
	Proposition         string  `json:"proposition"`
	Subdomain           uint32  `json:"subdomain"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
}

type MathVerificationResponse struct {
	Status            string  `json:"status"`
	Nonce             string  `json:"nonce"`
	ResultHash        string  `json:"result_hash"`
	DetokenizedOutput string  `json:"detokenized_output"`
	LogicIntegrity    float64 `json:"logic_integrity"`
	LatencyMs         float64 `json:"latency_ms"`
}

func VerifyMathDerivation(req MathVerificationRequest) (MathVerificationResponse, error) {
	subdomain := req.Subdomain
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}

	mapper := math.NewLaTeXMapper(subdomain)
	watchdog := math.NewInferenceWatchdog(subdomain)

	slots := mapper.MapLaTeXToTensor(req.Proposition, subdomain)
	validation := watchdog.ValidateMathStep(0, slots)

	response := MathVerificationResponse{
		Status:            "UNVERIFIED",
		Nonce:             fmt.Sprintf("0x%08x", slots[11]),
		ResultHash:        fmt.Sprintf("0x%08x", slots[3]),
		DetokenizedOutput: req.Proposition,
		LogicIntegrity:    validation.LogicIntegrity,
		LatencyMs:         0,
	}

	if validation.Valid {
		response.Status = "VERIFIED"
	}

	if !validation.Valid {
		response.Status = "REJECTED"
		response.ResultHash = validation.Error
	}

	return response, nil
}

func MineMathDataset(inputPath, outputPath string, subdomain uint32) (int, error) {
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}
	return miner.MineMathProofs(inputPath, outputPath, subdomain)
}

func HashFrame(latex string, subdomain uint32) string {
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}
	mapper := math.NewLaTeXMapper(subdomain)
	slots := mapper.MapLaTeXToTensor(latex, subdomain)

	result := ""
	for _, v := range slots {
		result += fmt.Sprintf("%08x", v)
	}
	return result
}

func HashFrameBytes(latex string, subdomain uint32) []byte {
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}
	mapper := math.NewLaTeXMapper(subdomain)
	slots := mapper.MapLaTeXToTensor(latex, subdomain)

	result := make([]byte, 48)
	for i, v := range slots {
		binary.BigEndian.PutUint32(result[i*4:], v)
	}
	return result
}

type MathVerifierService interface {
	Verify(latex string, subdomain uint32) *MathVerifyResponse
}

func RegisterMathVerifierService(grpcServer *grpc.Server, service MathVerifierService) {
	log.Println("Math Verifier Service registered (gRPC handler)")
}
