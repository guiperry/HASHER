package hasher

import (
	"bytes"
	"testing"
)

func TestNewHashNeuron(t *testing.T) {
	seed := [32]byte{0x01}
	neuron := NewHashNeuron(seed, "float")
	
	if neuron == nil {
		t.Fatal("NewHashNeuron returned nil")
	}
	if neuron.Seed != seed {
		t.Errorf("Expected seed %v, got %v", seed, neuron.Seed)
	}
	if neuron.OutputMode != "float" {
		t.Errorf("Expected output mode 'float', got '%s'", neuron.OutputMode)
	}
}

func TestHashNeuronForward(t *testing.T) {
	seed := [32]byte{0x01}
	neuron := NewHashNeuron(seed, "float")
	
	input := []byte("test input")
	result := neuron.Forward(input)
	
	if result < 0 || result > 1 {
		t.Errorf("Expected result in [0, 1], got %f", result)
	}
}

func TestNewHashNetwork(t *testing.T) {
	net, err := NewHashNetwork(784, 128, 64, 10)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	if net.InputSize != 784 {
		t.Errorf("Expected input size 784, got %d", net.InputSize)
	}
	if net.Hidden1 != 128 {
		t.Errorf("Expected hidden1 size 128, got %d", net.Hidden1)
	}
	if net.Hidden2 != 64 {
		t.Errorf("Expected hidden2 size 64, got %d", net.Hidden2)
	}
	if net.OutputSize != 10 {
		t.Errorf("Expected output size 10, got %d", net.OutputSize)
	}
	
	if len(net.Neurons1) != 128 {
		t.Errorf("Expected 128 neurons in hidden1, got %d", len(net.Neurons1))
	}
	if len(net.Neurons2) != 64 {
		t.Errorf("Expected 64 neurons in hidden2, got %d", len(net.Neurons2))
	}
	if len(net.NeuronsOut) != 10 {
		t.Errorf("Expected 10 neurons in output, got %d", len(net.NeuronsOut))
	}
}

func TestHashNetworkForward(t *testing.T) {
	net, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	input := make([]byte, 10)
	for i := range input {
		input[i] = byte(i)
	}
	
	output, err := net.Forward(input)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	
	if len(output) != 2 {
		t.Errorf("Expected 2 outputs, got %d", len(output))
	}
	
	for i, val := range output {
		if val < 0 || val > 1 {
			t.Errorf("Output %d: expected [0, 1], got %f", i, val)
		}
	}
}

func TestHashNetworkPredict(t *testing.T) {
	net, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	input := make([]byte, 10)
	for i := range input {
		input[i] = byte(i)
	}
	
	prediction, confidence, err := net.Predict(input)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	
	if prediction < 0 || prediction >= 2 {
		t.Errorf("Prediction out of range: %d", prediction)
	}
	
	if confidence < 0 || confidence > 1 {
		t.Errorf("Confidence out of range: %f", confidence)
	}
}

func TestHashNetworkSerialization(t *testing.T) {
	net1, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	data, err := net1.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	
	net2, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	
	if net2.InputSize != net1.InputSize ||
		net2.Hidden1 != net1.Hidden1 ||
		net2.Hidden2 != net1.Hidden2 ||
		net2.OutputSize != net1.OutputSize {
		t.Errorf("Network dimensions don't match after deserialization")
	}
	
	// Verify seeds are preserved
	for i := range net1.Seeds1 {
		if !bytes.Equal(net1.Seeds1[i][:], net2.Seeds1[i][:]) {
			t.Errorf("Seed1 %d doesn't match", i)
		}
	}
	for i := range net1.Seeds2 {
		if !bytes.Equal(net1.Seeds2[i][:], net2.Seeds2[i][:]) {
			t.Errorf("Seed2 %d doesn't match", i)
		}
	}
	for i := range net1.SeedsOut {
		if !bytes.Equal(net1.SeedsOut[i][:], net2.SeedsOut[i][:]) {
			t.Errorf("SeedOut %d doesn't match", i)
		}
	}
}

func TestRecursiveEngine(t *testing.T) {
	net, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	engine, err := NewRecursiveEngine(net, 5, 0.01, true)
	if err != nil {
		t.Fatalf("NewRecursiveEngine failed: %v", err)
	}
	
	input := make([]byte, 10)
	for i := range input {
		input[i] = byte(i)
	}
	
	result, err := engine.Infer(input)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	
	if result == nil {
		t.Fatal("Infer returned nil")
	}
	
	if len(result.Passes) != 5 {
		t.Errorf("Expected 5 passes, got %d", len(result.Passes))
	}
	
	if result.ValidPasses != 5 {
		t.Errorf("Expected 5 valid passes, got %d", result.ValidPasses)
	}
	
	if result.Consensus == nil {
		t.Fatal("No consensus result")
	}
	
	if result.Consensus.Prediction < 0 || result.Consensus.Prediction >= 2 {
		t.Errorf("Consensus prediction out of range: %d", result.Consensus.Prediction)
	}
	
	if result.Consensus.Confidence < 0 || result.Consensus.Confidence > 1 {
		t.Errorf("Consensus confidence out of range: %f", result.Consensus.Confidence)
	}
}

func TestRecursiveEngineWithoutSeedRotation(t *testing.T) {
	net, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	engine, err := NewRecursiveEngine(net, 3, 0.01, false)
	if err != nil {
		t.Fatalf("NewRecursiveEngine failed: %v", err)
	}
	
	input := make([]byte, 10)
	for i := range input {
		input[i] = byte(i)
	}
	
	result, err := engine.Infer(input)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	
	if len(result.Passes) != 3 {
		t.Errorf("Expected 3 passes, got %d", len(result.Passes))
	}
}

func TestLogicalValidator(t *testing.T) {
	validator, err := NewLogicalValidator()
	if err != nil {
		t.Fatalf("NewLogicalValidator failed: %v", err)
	}
	
	result, err := validator.Validate(5, "anomaly_detection", map[string]interface{}{
		"min_value": 0,
		"max_value": 10,
	})
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	
	if !result.Valid {
		t.Errorf("Expected valid prediction, got invalid")
	}
	
	if result.RulesApplied == 0 {
		t.Errorf("Expected rules to be applied")
	}
}

func TestValidationWithInvalidPrediction(t *testing.T) {
	validator, err := NewLogicalValidator()
	if err != nil {
		t.Fatalf("NewLogicalValidator failed: %v", err)
	}
	
	result, err := validator.Validate(-1, "anomaly_detection", map[string]interface{}{
		"min_value": 0,
		"max_value": 10,
	})
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	
	if result.Valid {
		t.Errorf("Expected invalid prediction, got valid")
	}
	
	if len(result.Errors) == 0 {
		t.Errorf("Expected validation errors")
	}
}

func TestLogicalRuleSerialization(t *testing.T) {
	rule, err := NewLogicalRule("constraint", []string{"prediction > 0"}, "Positive prediction", "Prediction must be positive")
	if err != nil {
		t.Fatalf("NewLogicalRule failed: %v", err)
	}
	
	data, err := rule.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	
	deserialized, err := DeserializeLogicalRule(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	
	if rule.RuleType != deserialized.RuleType {
		t.Errorf("Rule type mismatch")
	}
	if rule.Conclusion != deserialized.Conclusion {
		t.Errorf("Conclusion mismatch")
	}
	if len(rule.Premises) != len(deserialized.Premises) {
		t.Errorf("Premises count mismatch")
	}
	for i, premise := range rule.Premises {
		if premise != deserialized.Premises[i] {
			t.Errorf("Premise %d mismatch", i)
		}
	}
}

func BenchmarkHashNeuronForward(b *testing.B) {
	seed := [32]byte{0x01}
	neuron := NewHashNeuron(seed, "float")
	input := make([]byte, 64)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		neuron.Forward(input)
	}
}

func BenchmarkHashNetworkForward(b *testing.B) {
	net, _ := NewHashNetwork(784, 128, 64, 10)
	input := make([]byte, 784)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		net.Forward(input)
	}
}

func BenchmarkRecursiveEngineInfer(b *testing.B) {
	net, _ := NewHashNetwork(100, 64, 32, 10)
	engine, _ := NewRecursiveEngine(net, 21, 0.01, true)
	input := make([]byte, 100)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Infer(input)
	}
}

func TestKnowledgeBaseAddRule(t *testing.T) {
	kb, err := NewKnowledgeBase()
	if err != nil {
		t.Fatalf("NewKnowledgeBase failed: %v", err)
	}
	
	rule, err := NewLogicalRule("constraint", []string{"x > 0"}, "Positive value", "Value must be positive")
	if err != nil {
		t.Fatalf("NewLogicalRule failed: %v", err)
	}
	
	if err := kb.AddRule("test_domain", rule); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	
	rules, err := kb.GetRules("test_domain")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
	
	if rules[0].Conclusion != "Positive value" {
		t.Errorf("Rule conclusion mismatch")
	}
}

func TestKnowledgeBaseRemoveRule(t *testing.T) {
	kb, err := NewKnowledgeBase()
	if err != nil {
		t.Fatalf("NewKnowledgeBase failed: %v", err)
	}
	
	rule, err := NewLogicalRule("constraint", []string{"x > 0"}, "Positive value", "Value must be positive")
	if err != nil {
		t.Fatalf("NewLogicalRule failed: %v", err)
	}
	
	if err := kb.AddRule("test_domain", rule); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	
	if err := kb.RemoveRule("test_domain", 0); err != nil {
		t.Fatalf("RemoveRule failed: %v", err)
	}
	
	rules, err := kb.GetRules("test_domain")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules, got %d", len(rules))
	}
}

func TestRecursiveResultStatisticalSummary(t *testing.T) {
	net, err := NewHashNetwork(10, 4, 2, 2)
	if err != nil {
		t.Fatalf("NewHashNetwork failed: %v", err)
	}
	
	engine, err := NewRecursiveEngine(net, 5, 0.01, true)
	if err != nil {
		t.Fatalf("NewRecursiveEngine failed: %v", err)
	}
	
	input := make([]byte, 10)
	for i := range input {
		input[i] = byte(i)
	}
	
	result, err := engine.Infer(input)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	
	summary := result.StatisticalSummary()
	
	if summary.MeanConfidence < 0 || summary.MeanConfidence > 1 {
		t.Errorf("Mean confidence out of range: %f", summary.MeanConfidence)
	}
	
	if summary.ConfidenceStdDev < 0 {
		t.Errorf("Confidence std deviation out of range: %f", summary.ConfidenceStdDev)
	}
	
	if len(summary.ClassDistribution) == 0 {
		t.Errorf("Class distribution is empty")
	}
}
