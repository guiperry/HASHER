package storage

type WeightRecord struct {
	TokenID      int32   `json:"token_id"`
	BestSeed     string  `json:"best_seed"`
	FitnessScore float64 `json:"fitness_score"`
	Generation   int32   `json:"generation"`
	ContextKey   uint32  `json:"context_key"`
}
