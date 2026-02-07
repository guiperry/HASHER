package app

// DocumentRecord defines our ML-ready schema
type DocumentRecord struct {
	FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
	ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
	Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
	Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

// Config holds application configuration
type Config struct {
	InputDir     string
	OutputFile   string
	NumWorkers   int
	ChunkSize    int
	ChunkOverlap int
	OllamaModel  string
	OllamaHost   string
	CheckpointDB string
	BatchSize    int

	// Application data directories
	AppDataDir string
	DataDirs   map[string]string

	// arXiv mining configuration
	EnableArxivMining   bool
	ArxivCategories     []string
	ArxivMaxPapers      int
	ArxivRunInterval    string
	ArxivDownloadDelay  int
	ArxivBackgroundMode bool
	ArxivSortBy         string
	ArxivSortOrder      string

	// Script execution modes
	ProductionMode bool
	TestMode       bool
	OptimizedMode  bool
	HybridMode     bool
	NoArxivMode    bool
	DryRun         bool

	// Environment configuration
	CPUOverride      int
	CloudflareLimit  int
	GPUOverride      bool
	GPUOptimizations bool
}
