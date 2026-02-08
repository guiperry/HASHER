package config

type Config struct {
	Simulator  *SimulatorConfig  `json:"simulator"`
	Storage    *StorageConfig    `json:"storage"`
	Training   *TrainingConfig   `json:"training"`
	Deployment *DeploymentConfig `json:"deployment"`
	Validation *ValidationConfig `json:"validation"`
	Logging    *LoggingConfig    `json:"logging"`
}

type SimulatorConfig struct {
	DeviceType     string  `json:"device_type"`
	MaxConcurrency int     `json:"max_concurrency"`
	TargetHashRate float64 `json:"target_hash_rate"`
	CacheSize      int     `json:"cache_size"`
	GPUDevice      int     `json:"gpu_device"`
	Timeout        int     `json:"timeout"`
}

type StorageConfig struct {
	BasePath  string `json:"base_path"`
	LayerSize int    `json:"layer_size"`
}

type TrainingConfig struct {
	PopulationSize  int     `json:"population_size"`
	MaxGenerations  int     `json:"max_generations"`
	EliteRatio      float64 `json:"elite_ratio"`
	MutationRate    float64 `json:"mutation_rate"`
	TargetFitness   float64 `json:"target_fitness"`
	ValidationSplit float64 `json:"validation_split"`
}

type DeploymentConfig struct {
	BPFMapPath        string `json:"bpf_map_path"`
	OpenWRTEndpoint   string `json:"openwrt_endpoint"`
	DeploymentTimeout string `json:"deployment_timeout"`
	MaxRetries        int    `json:"max_retries"`
	RetryDelay        string `json:"retry_delay"`
	ValidationMode    string `json:"validation_mode"`
	BackupEnabled     bool   `json:"backup_enabled"`
	BackupPath        string `json:"backup_path"`
	RollbackEnabled   bool   `json:"rollback_enabled"`
}

type ValidationConfig struct {
	Timeout            string  `json:"timeout"`
	MaxConcurrency     int     `json:"max_concurrency"`
	RetryAttempts      int     `json:"retry_attempts"`
	ToleranceThreshold float64 `json:"tolerance_threshold"`
	EnableASIC         bool    `json:"enable_asic"`
}

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	Output     string `json:"output"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
}
