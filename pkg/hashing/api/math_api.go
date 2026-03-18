package api

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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

// NewMathVerifierServerWithRules creates a server whose watchdog uses schema-loaded rules.
func NewMathVerifierServerWithRules(subdomain uint32, rules []math.WatchdogRule) *MathVerifierServer {
	if subdomain == 0 {
		subdomain = math.SUBDOMAIN_ARITHMETIC
	}
	return &MathVerifierServer{
		watchdog:  math.NewWatchdogWithRules(subdomain, rules),
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
	} else if validation.Error != "" {
		response.Status = "REJECTED"
		response.ResultHash = validation.Error
	}

	return response
}

type MathVerifyResponse struct {
	Status            string  `json:"status"`
	Nonce             string  `json:"nonce"`
	ResultHash        string  `json:"result_hash"`
	DetokenizedOutput string  `json:"detokenized_output"`
	LogicIntegrity    float32 `json:"logic_integrity"`
	LatencyMs         float32 `json:"latency_ms"`
}

type ServerConfig struct {
	Port       string
	Subdomain  uint32
	SchemaPath string // optional: path to slot-schema YAML for rule loading
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:      ":50051",
		Subdomain: math.SUBDOMAIN_ARITHMETIC,
	}
}

// StartServer starts the HashNet Math Verification REST API on config.Port.
// Endpoints:
//
//	POST /v1/verify/math  — verify a LaTeX derivation step
//	GET  /health          — liveness probe
func StartServer(config *ServerConfig) error {
	if config == nil {
		config = DefaultServerConfig()
	}

	var server *MathVerifierServer

	if config.SchemaPath != "" {
		rules, err := math.LoadWatchdogRulesFromYAML(config.SchemaPath)
		if err != nil {
			log.Printf("Warning: could not load schema rules from %s: %v — using hard-coded fallback", config.SchemaPath, err)
			server = NewMathVerifierServer(config.Subdomain)
		} else {
			log.Printf("Loaded %d validation rules from %s", len(rules), config.SchemaPath)
			server = NewMathVerifierServerWithRules(config.Subdomain, rules)
		}
	} else {
		server = NewMathVerifierServer(config.Subdomain)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/verify/math", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req MathVerificationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		resp := server.Verify(req.Proposition, req.Subdomain)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"domain": "MATHASHER",
		})
	})

	log.Printf("HashNet Math Verification API listening on %s", config.Port)
	return http.ListenAndServe(config.Port, mux)
}

// VerifyHandler returns an http.HandlerFunc for POST /v1/verify/math.
// Mount on a gin router with gin.WrapF(server.VerifyHandler()).
func (s *MathVerifierServer) VerifyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req MathVerificationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		resp := s.Verify(req.Proposition, req.Subdomain)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding verify response: %v", err)
		}
	}
}

// MathHealthHandler returns an http.HandlerFunc for GET /v1/verify/math/health.
// Mount on a gin router with gin.WrapF(server.MathHealthHandler()).
func (s *MathVerifierServer) MathHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"domain": "MATHASHER",
		})
	}
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
	}

	if validation.Valid {
		response.Status = "VERIFIED"
	} else {
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
