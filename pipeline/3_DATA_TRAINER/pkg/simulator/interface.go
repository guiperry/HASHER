package simulator

type HashSimulator interface {
	SimulateHash(seed []byte, pass int) (uint32, error)
	ValidateSeed(seed []byte, targetToken int32) (bool, error)
	Shutdown() error
}
