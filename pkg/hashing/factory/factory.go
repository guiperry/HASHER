package factory

import (
	"fmt"
	"sort"
	"strings"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/hardware"
	"hasher/pkg/hashing/methods/asic"
	"hasher/pkg/hashing/methods/cuda"
	"hasher/pkg/hashing/methods/ebpf"
	"hasher/pkg/hashing/methods/software"
	"hasher/pkg/hashing/methods/ubpf"
)

// HashMethodConfig contains configuration for hash method selection
type HashMethodConfig struct {
	// Preferred method order (highest priority first)
	PreferredOrder []string `json:"preferred_order"`

	// Device-specific settings
	ASICDevice  string `json:"asic_device"`  // e.g., "/dev/bitmain-asic"
	CGMinerPath string `json:"cgminer_path"` // e.g., "/usr/local/bin/cgminer"
	CUDADevice  int    `json:"cuda_device"`  // GPU device number

	// Performance settings
	EnableFallback bool `json:"enable_fallback"` // Allow fallback to slower methods
	TrainingMode   bool `json:"training_mode"`   // Optimize for training vs production
}

// DefaultHashMethodConfig returns a sensible default configuration
func DefaultHashMethodConfig() *HashMethodConfig {
	return &HashMethodConfig{
		PreferredOrder: []string{
			"asic",     // 1. Direct ASIC device hashing
			"software", // 2. Software fallback
			"cuda",     // 3. CUDA simulator (training only)
			"ubpf",     // 4. uBPF simulation
			"ebpf",     // 5. eBPF OpenWRT (future)
		},
		ASICDevice:     "/dev/bitmain-asic",
		CGMinerPath:    "/opt/cgminer/cgminer",
		CUDADevice:     0,
		EnableFallback: true,
		TrainingMode:   false,
	}
}

// TrainingHashMethodConfig returns configuration optimized for training
func TrainingHashMethodConfig() *HashMethodConfig {
	config := DefaultHashMethodConfig()
	config.PreferredOrder = []string{
		"cuda",     // 1. CUDA for training performance
		"asic",     // 2. ASIC if available
		"software", // 3. Software fallback
		"ubpf",     // 4. uBPF simulation
		"ebpf",     // 5. eBPF OpenWRT (future)
	}
	config.TrainingMode = true
	return config
}

// HashMethodFactory creates and manages hash method instances
type HashMethodFactory struct {
	config   *HashMethodConfig
	methods  map[string]core.HashMethod
	best     core.HashMethod
	detected map[string]bool
}

// NewHashMethodFactory creates a new factory with the given configuration
func NewHashMethodFactory(config *HashMethodConfig) *HashMethodFactory {
	if config == nil {
		config = DefaultHashMethodConfig()
	}

	factory := &HashMethodFactory{
		config:   config,
		methods:  make(map[string]core.HashMethod),
		detected: make(map[string]bool),
	}

	factory.detectMethods()
	factory.selectBestMethod()

	return factory
}

// detectMethods performs hardware detection for all available methods
func (f *HashMethodFactory) detectMethods() {
	detector := hardware.NewDeviceDetector()
	detected := detector.DetectAvailableMethods()

	// Create and store methods based on detection results
	if detected["asic"] {
		asicMethod := asic.NewASICMethod(f.config.ASICDevice)
		f.methods["asic"] = asicMethod
		f.detected["asic"] = true
	} else {
		// Still create method but mark as unavailable
		asicMethod := asic.NewASICMethod(f.config.ASICDevice)
		f.methods["asic"] = asicMethod
		f.detected["asic"] = false
	}

	// Software method (always available)
	softwareMethod := software.NewSoftwareMethod()
	f.methods["software"] = softwareMethod
	f.detected["software"] = true

	// CUDA method
	if detected["cuda"] {
		cudaMethod := cuda.NewCudaMethod()
		f.methods["cuda"] = cudaMethod
		f.detected["cuda"] = true
	} else {
		cudaMethod := cuda.NewCudaMethod()
		f.methods["cuda"] = cudaMethod
		f.detected["cuda"] = false
	}

	// uBPF method
	if detected["ubpf"] {
		ubpfMethod := ubpf.NewUbpfMethod(f.config.ASICDevice, f.config.CGMinerPath)
		f.methods["ubpf"] = ubpfMethod
		f.detected["ubpf"] = true
	} else {
		ubpfMethod := ubpf.NewUbpfMethod(f.config.ASICDevice, f.config.CGMinerPath)
		f.methods["ubpf"] = ubpfMethod
		f.detected["ubpf"] = false
	}

	// eBPF method (future - always unavailable for now)
	ebpfMethod := ebpf.NewEbpfMethod()
	f.methods["ebpf"] = ebpfMethod
	f.detected["ebpf"] = false
}

// selectBestMethod chooses the best available method based on configuration
func (f *HashMethodFactory) selectBestMethod() {
	for _, methodName := range f.config.PreferredOrder {
		if method, exists := f.methods[methodName]; exists {
			if method.IsAvailable() {
				f.best = method
				return
			}
		}
	}

	// If no preferred method is available, fall back to software
	if softwareMethod, exists := f.methods["software"]; exists {
		f.best = softwareMethod
	}
}

// GetBestMethod returns the currently selected best hashing method
func (f *HashMethodFactory) GetBestMethod() core.HashMethod {
	return f.best
}

// GetMethod returns a specific hashing method by name
func (f *HashMethodFactory) GetMethod(name string) core.HashMethod {
	if method, exists := f.methods[name]; exists {
		return method
	}
	return nil
}

// GetAllMethods returns all available hashing methods
func (f *HashMethodFactory) GetAllMethods() map[string]core.HashMethod {
	result := make(map[string]core.HashMethod)
	for name, method := range f.methods {
		result[name] = method
	}
	return result
}

// GetAvailableMethods returns all available hashing methods
func (f *HashMethodFactory) GetAvailableMethods() map[string]core.HashMethod {
	result := make(map[string]core.HashMethod)
	for name, method := range f.methods {
		if method.IsAvailable() {
			result[name] = method
		}
	}
	return result
}

// GetDetectionReport returns a report of detected methods and their status
func (f *HashMethodFactory) GetDetectionReport() *DetectionReport {
	report := &DetectionReport{
		Methods:        make([]*MethodStatus, 0),
		BestMethod:     "none",
		TotalMethods:   len(f.methods),
		AvailableCount: 0,
	}

	// Get method names sorted by priority
	methodNames := make([]string, 0, len(f.methods))
	for _, name := range f.config.PreferredOrder {
		if _, exists := f.methods[name]; exists {
			methodNames = append(methodNames, name)
		}
	}

	// Add any methods not in preferred order
	for name := range f.methods {
		found := false
		for _, preferred := range f.config.PreferredOrder {
			if name == preferred {
				found = true
				break
			}
		}
		if !found {
			methodNames = append(methodNames, name)
		}
	}

	// Build status for each method
	for _, name := range methodNames {
		method := f.methods[name]
		available := f.detected[name]
		caps := method.GetCapabilities()

		status := &MethodStatus{
			Name:         name,
			Available:    available,
			Priority:     f.getPriority(name),
			Capabilities: caps,
			Description:  f.getMethodDescription(name),
		}

		report.Methods = append(report.Methods, status)

		if available {
			report.AvailableCount++
		}
	}

	// Determine best method
	if f.best != nil {
		report.BestMethod = f.best.Name()
	}

	return report
}

// getPriority returns the priority index of a method
func (f *HashMethodFactory) getPriority(name string) int {
	for i, preferred := range f.config.PreferredOrder {
		if name == preferred {
			return i
		}
	}
	return 999 // Low priority for methods not in preferred list
}

// getMethodDescription returns a human-readable description for a method
func (f *HashMethodFactory) getMethodDescription(name string) string {
	descriptions := map[string]string{
		"asic":     "Direct ASIC device hashing via USB (/dev/bitmain-asic)",
		"software": "Pure Go software fallback using crypto/sha256",
		"cuda":     "CUDA GPU acceleration for training pipeline only",
		"ubpf":     "uBPF simulation with USB and CGMiner API support",
		"ebpf":     "eBPF OpenWRT kernel (future - requires ASIC flash)",
	}

	if desc, exists := descriptions[name]; exists {
		return desc
	}
	return "Unknown hashing method"
}

// InitializeBestMethod initializes the selected best method
func (f *HashMethodFactory) InitializeBestMethod() error {
	if f.best == nil {
		return fmt.Errorf("no method selected")
	}
	return f.best.Initialize()
}

// ShutdownAll shuts down all methods
func (f *HashMethodFactory) ShutdownAll() error {
	var errors []string

	for name, method := range f.methods {
		if err := method.Shutdown(); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ReinitializeDetection re-runs hardware detection and method selection
func (f *HashMethodFactory) ReinitializeDetection() {
	f.ShutdownAll()
	f.detectMethods()
	f.selectBestMethod()
}

// DetectionReport contains the results of hardware detection
type DetectionReport struct {
	Methods        []*MethodStatus `json:"methods"`
	BestMethod     string          `json:"best_method"`
	TotalMethods   int             `json:"total_methods"`
	AvailableCount int             `json:"available_count"`
}

// MethodStatus describes the status of a single hashing method
type MethodStatus struct {
	Name         string             `json:"name"`
	Available    bool               `json:"available"`
	Priority     int                `json:"priority"`
	Capabilities *core.Capabilities `json:"capabilities"`
	Description  string             `json:"description"`
}

// SortMethodsByPriority sorts methods by priority (helper for reports)
func SortMethodsByPriority(methods []*MethodStatus) {
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Priority < methods[j].Priority
	})
}
