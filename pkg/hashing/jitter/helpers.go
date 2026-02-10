package jitter

// HelperRegistry manages BPF helper functions for uBPF integration
// This provides the bridge between Go and the uBPF VM for flash search operations

// HelperID represents the ID of a BPF helper function
type HelperID uint32

const (
	// HelperID_CUDA_Bridge is the ID for the CUDA/ASIC bridge helper
	HelperID_CUDA_Bridge HelperID = 1

	// HelperID_FlashSearch is the ID for the flash search helper
	HelperID_FlashSearch HelperID = 2

	// HelperID_HashCompute is the ID for hash computation helper
	HelperID_HashCompute HelperID = 3

	// HelperID_JitterApply is the ID for jitter application helper
	HelperID_JitterApply HelperID = 4
)

// HelperFunction represents a BPF helper function signature
// All BPF helpers take 5 uint64 arguments and return uint64
type HelperFunction func(arg1, arg2, arg3, arg4, arg5 uint64) uint64

// HelperRegistry maintains a mapping of helper IDs to functions
type HelperRegistry struct {
	helpers map[HelperID]HelperFunction
	engine  *JitterEngine
}

// NewHelperRegistry creates a new helper registry with the given jitter engine
func NewHelperRegistry(engine *JitterEngine) *HelperRegistry {
	registry := &HelperRegistry{
		helpers: make(map[HelperID]HelperFunction),
		engine:  engine,
	}

	// Register default helpers
	registry.registerDefaultHelpers()

	return registry
}

// Register registers a helper function with the given ID
func (hr *HelperRegistry) Register(id HelperID, fn HelperFunction) {
	hr.helpers[id] = fn
}

// Get retrieves a helper function by ID
func (hr *HelperRegistry) Get(id HelperID) (HelperFunction, bool) {
	fn, exists := hr.helpers[id]
	return fn, exists
}

// Call invokes a helper function by ID with the given arguments
func (hr *HelperRegistry) Call(id HelperID, arg1, arg2, arg3, arg4, arg5 uint64) uint64 {
	fn, exists := hr.helpers[id]
	if !exists {
		return 0 // Return 0 for unregistered helpers
	}
	return fn(arg1, arg2, arg3, arg4, arg5)
}

// registerDefaultHelpers registers the standard helper functions
func (hr *HelperRegistry) registerDefaultHelpers() {
	// Register Flash Search helper (ID 2)
	hr.Register(HelperID_FlashSearch, hr.flashSearchHelper)

	// Register Hash Compute helper (ID 3)
	hr.Register(HelperID_HashCompute, hr.hashComputeHelper)

	// Register Jitter Apply helper (ID 4)
	hr.Register(HelperID_JitterApply, hr.jitterApplyHelper)

	// Note: CUDA Bridge (ID 1) must be registered separately with platform-specific implementation
}

// flashSearchHelper implements the flash search BPF helper
// arg1: hash value (first 4 bytes of hash) to lookup
// returns: jitter vector (uint32 encoded in uint64)
func (hr *HelperRegistry) flashSearchHelper(arg1, _, _, _, _ uint64) uint64 {
	hashKey := uint32(arg1)

	jitter, found := hr.engine.searcher.Search(hashKey)
	if !found {
		jitter = hr.engine.searcher.GenerateDefaultJitter(hashKey)
	}

	return uint64(jitter)
}

// hashComputeHelper implements the hash computation BPF helper
// arg1: pointer to data (64-bit address)
// arg2: length of data
// returns: first 4 bytes of hash result
func (hr *HelperRegistry) hashComputeHelper(arg1, arg2, _, _, _ uint64) uint64 {
	// In a real implementation with actual memory access:
	// data := (*[1 << 30]byte)(unsafe.Pointer(uintptr(arg1)))[:arg2:arg2]
	// hash, _ := hr.engine.hashMethod.ComputeDoubleHash(data)
	// return uint64(ExtractLookupKey(hash))

	// For now, return a placeholder
	return 0
}

// jitterApplyHelper implements the jitter application BPF helper
// arg1: pointer to 80-byte header
// arg2: jitter value to apply
// returns: 0 on success, 1 on error
func (hr *HelperRegistry) jitterApplyHelper(arg1, arg2, _, _, _ uint64) uint64 {
	// In a real implementation with actual memory access:
	// header := (*[80]byte)(unsafe.Pointer(uintptr(arg1)))
	// jitter := JitterVector(arg2)
	// err := XORJitterIntoHeader(header[:], jitter)
	// if err != nil {
	//     return 1
	// }
	// return 0

	// For now, return success
	return 0
}

// ExportFlashSearchCallback exports the flash search function for CGO
// This function signature matches what uBPF expects for external functions
//
//export FlashSearchCallback
func FlashSearchCallback(hashVal uint64) uint64 {
	// This is a placeholder for the actual CGO export
	// In production, this would be called by the uBPF VM
	return uint64(JitterVector(0xDEADBEEF))
}

// UBPFHelperAdapter provides an adapter for uBPF VM integration
type UBPFHelperAdapter struct {
	registry *HelperRegistry
}

// NewUBPFHelperAdapter creates a new adapter for uBPF integration
func NewUBPFHelperAdapter(engine *JitterEngine) *UBPFHelperAdapter {
	return &UBPFHelperAdapter{
		registry: NewHelperRegistry(engine),
	}
}

// GetHelperCount returns the number of registered helpers
func (ua *UBPFHelperAdapter) GetHelperCount() int {
	return len(ua.registry.helpers)
}

// GetHelperNames returns a map of helper IDs to names
func (ua *UBPFHelperAdapter) GetHelperNames() map[HelperID]string {
	return map[HelperID]string{
		HelperID_CUDA_Bridge: "cuda_bridge",
		HelperID_FlashSearch: "flash_search",
		HelperID_HashCompute: "hash_compute",
		HelperID_JitterApply: "jitter_apply",
	}
}

// ExecuteHelper executes a helper by ID (for testing and simulation)
func (ua *UBPFHelperAdapter) ExecuteHelper(id HelperID, args ...uint64) uint64 {
	if len(args) < 5 {
		// Pad with zeros
		padded := make([]uint64, 5)
		copy(padded, args)
		args = padded
	}
	return ua.registry.Call(id, args[0], args[1], args[2], args[3], args[4])
}
