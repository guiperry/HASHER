package simulator

type HashSimulator interface {
	SimulateHash(seed []byte, pass int) (uint32, error)
	SimulateBitcoinHeader(header []byte) (uint32, error) // BM1382 "Camouflage" support
	RecursiveMine(header []byte, passes int) (uint32, error) // 21-pass loop with jitter
	ValidateSeed(seed []byte, targetToken int32) (bool, error)
	Shutdown() error
}
