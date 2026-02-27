package app

// DocumentRecord defines our ML-ready schema
type DocumentRecord struct {
	FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
	ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
	Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
	Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`

	// NLP Metadata (from NLP Bridge)
	Tokens       []string `parquet:"name=tokens, type=LIST, valuetype=BYTE_ARRAY, valueconvertedtype=UTF8"`
	TokenOffsets []int32  `parquet:"name=token_offsets, type=LIST, valuetype=INT32"`
	POSTags      []int    `parquet:"name=pos_tags, type=LIST, valuetype=INT32"`
	Tenses       []int    `parquet:"name=tenses, type=LIST, valuetype=INT32"`
	DepHashes    []uint32 `parquet:"name=dep_hashes, type=LIST, valuetype=INT32"`
}

// AlpacaRecord defines the standard instruction-tuning format
type AlpacaRecord struct {
	Instruction string `json:"instruction"`
	Input       string `json:"input"`
	Output      string `json:"output"`
}

// AlpacaDocumentRecord combines Alpaca format with our ML-ready metadata
type AlpacaDocumentRecord struct {
	AlpacaRecord
	FileName  string    `json:"file_name"`
	ChunkID   int32     `json:"chunk_id"`
	Content   string    `json:"content"`
	Embedding []float32 `json:"embedding"`

	// NLP Metadata (from NLP Bridge)
	Tokens       []string `json:"tokens"`
	TokenOffsets []int32  `json:"token_offsets"`
	POSTags      []int    `json:"pos_tags"`
	Tenses       []int    `json:"tenses"`
	DepHashes    []uint32 `json:"dep_hashes"`
}

// Config holds application configuration
type Config struct {
	InputDir     string
	OutputFile   string
	NumWorkers   int
	ChunkSize    int
	ChunkOverlap int
	OllamaModel  string
	OllamaGenModel string
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
	GoatMode       bool
	DemoMode       bool
	DryRun         bool

	// Environment configuration
	CPUOverride        int
	CloudflareLimit    int
	GPUOverride        bool
	GPUOptimizations   bool
	CloudflareEndpoint string
}
