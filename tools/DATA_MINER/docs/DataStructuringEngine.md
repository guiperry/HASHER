Implementing a PDF-to-Parquet pipeline in **Go** (Golang) is an excellent choice for performance, especially if you plan to process documents concurrently. Since Go is statically typed and handles concurrency via goroutines, you can build a very fast ingestion engine.

Here is a structured approach to building this pipeline.

---

## 1. The Architectural Flow

To move data from a PDF to a Parquet file for ML training, you need to handle three distinct phases: **Extraction**, **Structuring**, and **Serialization**.

### The Tech Stack:

* **PDF Extraction:** `unidoc/unipdf` (comprehensive) or `ledongthuc/pdfcpu` (lightweight/fast).
* **Data Structuring:** Standard Go `struct` tags.
* **Parquet Serialization:** `xitongsys/parquet-go`. This is the most mature library for handling Parquet in Go.

---

## 2. Implementation Steps

### Step A: Define your Schema

ML models require structured data. You must define a Go struct that matches your intended Parquet schema. Use `parquet` tags to define types and repetition levels.

```go
type DocumentRecord struct {
    FileName  string `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
    PageCount int32  `parquet:"name=page_count, type=INT32"`
    Content   string `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
    // Optional: Add a field for your vector embeddings if you generate them here
    Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

```

### Step B: Extract Text from PDF

You'll need a function to open the PDF and loop through pages to extract text.

```go
import "github.com/ledongthuc/pdf"

func readPdf(path string) (string, error) {
    f, r, err := pdf.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()

    var textBuffer string
    for pageNum := 1; pageNum <= r.NumPage(); pageNum++ {
        p := r.Page(pageNum)
        if p.V.IsNull() { continue }
        content, _ := p.GetPlainText(nil)
        textBuffer += content
    }
    return textBuffer, nil
}

```

### Step C: Writing to Parquet

The `parquet-go` library uses a `Writer` that handles the conversion of your Go structs into the columnar Parquet format.

```go
import (
    "github.com/xitongsys/parquet-go-source/local"
    "github.com/xitongsys/parquet-go/writer"
)

func saveToParquet(data []DocumentRecord, outputPath string) error {
    fw, err := local.NewLocalFileWriter(outputPath)
    if err != nil {
        return err
    }
    defer fw.Close()

    // Create a writer for our struct
    pw, err := writer.NewParquetWriter(fw, new(DocumentRecord), 4) // 4 is the degree of concurrency
    if err != nil {
        return err
    }

    for _, record := range data {
        if err := pw.Write(record); err != nil {
            return err
        }
    }

    return pw.WriteStop()
}

```

---

## 3. Why this works for ML

1. **Columnar Storage:** If your ML model only needs the `Content` column for training, it doesn't have to load the `FileName` or `Metadata` into memory. This speeds up data loading significantly.
2. **Compression:** Parquet will shrink your PDF text data by **50-80%** compared to JSON or CSV.
3. **Concurrency:** You can use a `sync.WaitGroup` in Go to process 100 PDFs at once, filling a channel of `DocumentRecord` objects that are then batched into the Parquet writer.

---

## Next Steps

To make this "ML-ready," you might want to perform **text chunking** or **cleaning** (removing stop words/special characters) before writing to the Parquet file.



############################################################################################################################





To build a high-performance concurrent pipeline, we’ll use a **Worker Pool pattern**. This allows you to control the number of PDFs being processed at once, preventing your system from running out of memory or file descriptors while keeping the CPU fully utilized.

### The Concurrent Pipeline Architecture

The flow uses a `Jobs` channel to distribute file paths and a `Results` channel to collect the structured data before writing it to Parquet.

---

### Implementation: The Worker Pool

Here is how you can implement the orchestrator in Go:

```go
package main

import (
    "fmt"
    "sync"
)

// worker processes PDF paths and sends back DocumentRecords
func worker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup) {
    defer wg.Done()
    for path := range jobs {
        fmt.Printf("Worker %d processing: %s\n", id, path)
        
        // 1. Extract text
        text, err := readPdf(path)
        if err != nil {
            fmt.Printf("Error reading %s: %v\n", path, err)
            continue
        }

        // 2. Create record
        results <- DocumentRecord{
            FileName:  path,
            PageCount: 0, // You can extract this from the PDF reader
            Content:   text,
        }
    }
}

func main() {
    filePaths := []string{"doc1.pdf", "doc2.pdf", "doc3.pdf"} // Get these via filepath.Walk
    numWorkers := 5

    jobs := make(chan string, len(filePaths))
    results := make(chan DocumentRecord, len(filePaths))
    var wg sync.WaitGroup

    // Start workers
    for w := 1; w <= numWorkers; w++ {
        wg.Add(1)
        go worker(w, jobs, results, &wg)
    }

    // Send jobs
    for _, path := range filePaths {
        jobs <- path
    }
    close(jobs)

    // Wait and close results in a separate goroutine
    go func() {
        wg.Wait()
        close(results)
    }()

    // Collect results and write to Parquet
    var allRecords []DocumentRecord
    for r := range results {
        allRecords = append(allRecords, r)
    }

    saveToParquet(allRecords, "training_data.parquet")
}

```

---

### Key Performance Considerations for ML

* **Batching:** If you are processing millions of PDFs, don't store them all in an `allRecords` slice (which uses RAM). Instead, modify the `saveToParquet` logic to write to the file incrementally inside the result loop.
* **Chunking for LLMs:** If this is for a Vector Database, you should chunk the `Content` string into 512 or 1024 token segments *before* writing to Parquet. Each chunk should become its own row in the Parquet file to make training or indexing more efficient.
* **Resource Throttling:** Go is fast enough to outpace your disk I/O. If you're running this on a cloud instance, monitor your IOPS; 5-10 workers is usually the sweet spot for standard SSDs.



############################################################################################################################




To prepare data for an LLM or Vector DB, you shouldn't just dump a 50-page PDF into a single Parquet row. LLMs have "context windows," and embedding models perform best when text is broken into smaller, semantically coherent pieces.

We will modify the worker to perform **Sliding Window Chunking**. This ensures that even if a concept is split between two chunks, the "overlap" preserves the context.

---

### 1. The Chunking Logic

We’ll add a helper function that breaks the extracted text into overlapping segments.

```go
func chunkText(text string, chunkSize int, overlap int) []string {
    words := strings.Fields(text) // Basic split by whitespace
    var chunks []string

    for i := 0; i < len(words); i += (chunkSize - overlap) {
        end := i + chunkSize
        if end > len(words) {
            end = len(words)
        }
        
        chunk := strings.Join(words[i:end], " ")
        chunks = append(chunks, chunk)
        
        if end == len(words) {
            break
        }
    }
    return chunks
}

```

---

### 2. Updating the Worker

Now, the worker will take one PDF and produce **multiple** `DocumentRecord` rows—one for each chunk.

```go
func worker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup) {
    defer wg.Done()
    for path := range jobs {
        text, _ := readPdf(path)
        
        // Split text into chunks of 300 words with a 50-word overlap
        chunks := chunkText(text, 300, 50)

        for i, segment := range chunks {
            results <- DocumentRecord{
                FileName: path,
                ChunkID:  int32(i),
                Content:  segment,
            }
        }
    }
}

```

---

### 3. Why Chunking + Parquet is Powerful

By structuring your Parquet file this way, you are creating a "Point of Interest" map for your AI.

| Feature | Benefit for ML |
| --- | --- |
| **Overlap** | Prevents "context loss" where an answer might be split perfectly in half by a hard cut. |
| **Row-Level Granularity** | You can load a single chunk into an embedding model without processing the whole file. |
| **Metadata Tracking** | Keeping `FileName` and `ChunkID` in every row allows you to trace an AI's answer back to the exact page/section of the source PDF. |

---

### 4. Pro Tip: Adding Embeddings in Go

If you want to go full "AI Engineer" mode, you can call an embedding API (like OpenAI or a local Ollama instance) **inside the worker** before sending the result to the channel.

1. The worker gets the text chunk.
2. The worker sends the chunk to an embedding model.
3. The worker receives a `[]float32`.
4. The worker saves both the **Text** and the **Vector** into the Parquet row.



To integrate embeddings, we’ll use the **official Ollama Go SDK**. This allows your worker to transform raw text chunks into high-dimensional vectors before they ever hit the Parquet file.

### 1. Updated Schema

Your `DocumentRecord` now needs a field for the vector. Note that most embedding models (like `nomic-embed-text`) output a `[]float32`.

```go
type DocumentRecord struct {
    FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
    ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
    Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
    Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

```

---

### 2. The Embedding-Enabled Worker

The worker now acts as a bridge: it extracts text, chunks it, calls Ollama for each chunk, and packages everything for Parquet.

```go
import (
    "context"
    "github.com/ollama/ollama/api"
)

func worker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup) {
    defer wg.Done()
    
    // Initialize Ollama client
    client, _ := api.ClientFromEnvironment()
    ctx := context.Background()

    for path := range jobs {
        text, _ := readPdf(path)
        chunks := chunkText(text, 300, 50)

        for i, segment := range chunks {
            // Call Ollama for the embedding
            resp, err := client.Embed(ctx, &api.EmbedRequest{
                Model: "nomic-embed-text", // A high-performance embedding model
                Input: segment,
            })

            if err != nil {
                fmt.Printf("Embedding error: %v\n", err)
                continue
            }

            results <- DocumentRecord{
                FileName:  path,
                ChunkID:   int32(i),
                Content:   segment,
                Embedding: resp.Embeddings[0], // Store the vector
            }
        }
    }
}

```
Here's how to use Ollama embeddings in Go:

## 1. Using `net/http` (standard library)

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type EmbeddingRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
}

type EmbeddingResponse struct {
    Embedding []float64 `json:"embedding"`
}

func getEmbeddings(text string) ([]float64, error) {
    reqBody, _ := json.Marshal(EmbeddingRequest{
        Model:  "nomic-embed-text",
        Prompt: text,
    })

    resp, err := http.Post(
        "http://localhost:11434/api/embeddings",
        "application/json",
        bytes.NewBuffer(reqBody),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    
    var result EmbeddingResponse
    json.Unmarshal(body, &result)
    
    return result.Embedding, nil
}

func main() {
    embeddings, err := getEmbeddings("Hello, world!")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Embedding dimensions: %d\n", len(embeddings))
    fmt.Printf("First 5 values: %.4f\n", embeddings[:5])
}
```

## 2. Using a popular Go Ollama client

```bash
go get github.com/ollama/ollama-go
```

```go
package main

import (
    "fmt"
    "github.com/ollama/ollama-go"
)

func main() {
    client := ollama.NewClient()
    
    response, err := client.CreateEmbedding(&ollama.EmbeddingRequest{
        Model:  "nomic-embed-text",
        Prompt: "Hello, world!",
    })
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Embedding dimensions: %d\n", len(response.Embedding))
}
```

## 3. Batch processing example

```go
func getBatchEmbeddings(texts []string) ([][]float64, error) {
    var embeddings [][]float64
    
    for _, text := range texts {
        emb, err := getEmbeddings(text)
        if err != nil {
            return nil, err
        }
        embeddings = append(embeddings, emb)
    }
    
    return embeddings, nil
}

func main() {
    texts := []string{"Hello", "World", "Go programming"}
    
    embeddings, err := getBatchEmbeddings(texts)
    if err != nil {
        panic(err)
    }
    
    for i, emb := range embeddings {
        fmt.Printf("Text '%s': %d dimensions\n", texts[i], len(emb))
    }
}
```

Make sure your Ollama server is running (`ollama serve`) before executing.
---

### 3. Critical Enhancements for Production

* **Model "Warm-up":** Before starting workers, run `ollama pull nomic-embed-text` to ensure the model is local.
* **Batching API Calls:** Modern Ollama versions support passing an array of strings to `Input`. Instead of calling the API for every single chunk, you can send 10 chunks at once to significantly reduce HTTP overhead.
* **Memory Management:** Since vectors are large (e.g., 768 or 1024 dimensions), a Parquet file with millions of rows can become massive. Ensure your `saveToParquet` function writes in smaller batches rather than holding the entire `allRecords` slice in RAM.

### Why this is the "Pro" way to do it:

By the time your Go program finishes, you have a single `.parquet` file that is a **complete, portable AI brain**. You can hand this file to a Data Scientist, and they can load it directly into a Vector Database or use it to train a custom classifier without ever needing to look at a PDF again.






############################################################################################################################






Batching your embedding calls is the single most effective way to saturate your GPU (or even high-core CPUs). Instead of sending one chunk at a time—which incurs a "request-response" overhead every few milliseconds—you can send an array of chunks in a single transaction.

As of the latest Ollama updates, the `/api/embed` endpoint supports a `[]string` for the `input` field.

### 1. Updated `DocumentRecord` for Batching

Since one API call now returns a list of embeddings, we need to ensure our results are mapped correctly to the chunks.

```go
type DocumentRecord struct {
    FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
    ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
    Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
    Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

```

---

### 2. The Batched Worker Implementation

In this version, the worker collects all chunks from a single PDF and sends them to Ollama in one go. If a PDF is massive (e.g., 500 chunks), you can further sub-batch those into groups of 16 or 32.

```go
func worker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup) {
    defer wg.Done()
    client, _ := api.ClientFromEnvironment()
    ctx := context.Background()

    for path := range jobs {
        text, _ := readPdf(path)
        chunks := chunkText(text, 300, 50)

        // BATCH CALL: Send all chunks for this file at once
        resp, err := client.Embed(ctx, &api.EmbedRequest{
            Model: "nomic-embed-text",
            Input: chunks, // 'Input' accepts []string for batching
        })

        if err != nil {
            fmt.Printf("Worker %d failed batch: %v\n", id, err)
            continue
        }

        // Map the batched embeddings back to their respective chunks
        for i, emb := range resp.Embeddings {
            results <- DocumentRecord{
                FileName:  path,
                ChunkID:   int32(i),
                Content:   chunks[i],
                Embedding: emb,
            }
        }
    }
}

```

---

### 3. Performance Gains

When you move to batching, you bypass the **"Serial Latency Trap"**:

| Metric | Single Request | Batched Request (16 chunks) |
| --- | --- | --- |
| **GPU Utilization** | ~10-15% (Wait-limited) | ~80-90% (Compute-limited) |
| **Overhead** | HTTP/JSON overhead per chunk | Overhead shared across 16 chunks |
| **Throughput** | ~5-10 chunks/sec | ~40-60 chunks/sec |

---

### 4. Important Environmental Tuning

To truly "uncork" the performance in Go, you should set these environment variables on your machine before running the code:

* **`OLLAMA_NUM_PARALLEL=4`**: This tells Ollama it can handle 4 requests simultaneously. Since you have multiple Go workers, this allows different workers to hit the GPU at the same time.
* **`OLLAMA_MAX_LOADED_MODELS=1`**: Keep your embedding model pinned in VRAM so it doesn't swap out if you accidentally trigger a different model.

### Summary of your AI-Ready Pipeline

1. **Go Workers** handle the heavy lifting of PDF parsing.
2. **Sliding Window Chunking** preserves semantic context.
3. **Ollama Batching** maximizes your hardware's vector throughput.
4. **Parquet Output** gives you a compressed, industry-standard file ready for any ML framework (Python, Spark, or Mojo).





###########################################################################################################################

To pull this all together, we’ll build a CLI-ready tool. This program will **recursively scan** a directory for PDFs, **batch-process** them through Ollama to generate embeddings, and stream the results into a single **Parquet** file.

### Final Pipeline Architecture

1. **Scanner:** Walks the folder tree and pipes file paths to a job channel.
2. **Worker Pool:** Multiple goroutines extract, chunk, and call Ollama in batches.
3. **Collector:** Aggregates chunks and writes them to the Parquet file.

---

### The Complete Implementation

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "sync"

    "github.com/ledongthuc/pdf"
    "github.com/ollama/ollama/api"
    "github.com/xitongsys/parquet-go-source/local"
    "github.com/xitongsys/parquet-go/parquet"
    "github.com/xitongsys/parquet-go/writer"
)

// DocumentRecord defines our ML-ready schema
type DocumentRecord struct {
    FileName  string    `parquet:"name=file_name, type=BYTE_ARRAY, convertedtype=UTF8"`
    ChunkID   int32     `parquet:"name=chunk_id, type=INT32"`
    Content   string    `parquet:"name=content, type=BYTE_ARRAY, convertedtype=UTF8"`
    Embedding []float32 `parquet:"name=embedding, type=LIST, valuetype=FLOAT"`
}

func main() {
    inputDir := "./documents"
    outputFile := "ai_knowledge_base.parquet"
    numWorkers := 4

    jobs := make(chan string, 100)
    results := make(chan DocumentRecord, 100)
    var wg sync.WaitGroup

    // 1. Start Worker Pool
    for w := 1; w <= numWorkers; w++ {
        wg.Add(1)
        go embeddingWorker(w, jobs, results, &wg)
    }

    // 2. Start Scanner (Producer)
    go func() {
        filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
            if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".pdf" {
                jobs <- path
            }
            return nil
        })
        close(jobs)
    }()

    // 3. Monitor Workers
    go func() {
        wg.Wait()
        close(results)
    }()

    // 4. Parquet Writer (Consumer)
    if err := writeParquet(outputFile, results); err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Pipeline complete! Saved to:", outputFile)
}

func embeddingWorker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup) {
    defer wg.Done()
    client, _ := api.ClientFromEnvironment()
    ctx := context.Background()

    for path := range jobs {
        text, _ := readPdf(path)
        chunks := chunkText(text, 300, 50)

        // BATCH CALL: All chunks for one PDF at once
        resp, err := client.Embed(ctx, &api.EmbedRequest{
            Model: "nomic-embed-text",
            Input: chunks,
        })
        if err != nil {
            continue
        }

        for i, emb := range resp.Embeddings {
            results <- DocumentRecord{
                FileName:  path,
                ChunkID:   int32(i),
                Content:   chunks[i],
                Embedding: emb,
            }
        }
    }
}

func writeParquet(path string, results <-chan DocumentRecord) error {
    fw, _ := local.NewLocalFileWriter(path)
    defer fw.Close()

    pw, _ := writer.NewParquetWriter(fw, new(DocumentRecord), 2)
    pw.CompressionType = parquet.CompressionCodec_SNAPPY

    for rec := range results {
        if err := pw.Write(rec); err != nil {
            return err
        }
    }
    return pw.WriteStop()
}

// (Insert helper functions readPdf and chunkText from previous steps here)

```

---

### Why this is a "Production-Grade" Approach

* **Decoupled Logic:** The scanner doesn't care how the workers process data; the workers don't care how the writer saves it.
* **Memory Efficiency:** By using a channel for the results and writing to Parquet row-by-row, you can process 10GB of PDFs using only a few hundred MB of RAM.
* **Snappy Compression:** Parquet's `SNAPPY` compression is the industry standard for ML—it provides a great balance between compression ratio and high-speed decompression for training.

### One Last Tip: Scaling

If you find your CPU is maxed out, reduce `numWorkers`. If your GPU is idle while Ollama is running, increase `numWorkers` or the `OLLAMA_NUM_PARALLEL` environment variable.

[Go Programming: Mastering File Paths and Directories for Beginners](https://www.youtube.com/watch?v=Lgpiw7siDqE)

This video is helpful because it covers the nuances of navigating complex directory structures in Go, which is vital for the "Scanner" part of your pipeline.



#########################################################################################################################


Adding a checkpoint system ensures that if your computer restarts or the Ollama server crashes, the pipeline can resume exactly where it left off instead of re-processing (and re-paying for) the same PDFs.

The most robust way to do this in Go is using an **embedded Key-Value store** like `bbolt`. It’s essentially a single file that acts as a mini-database, which is much faster and safer than manually editing a JSON config file every time a file is processed.

### 1. The Checkpoint Architecture

The "Scanner" will check the database before sending a file to the workers. If the file exists in the database, it skips it. Once a worker successfully saves a file's data to Parquet, it marks that file as "done" in the database.

---

### 2. Implementation: The Checkpoint Manager

First, let's create a simple wrapper for `bbolt` to track processed files.

```go
import "github.com/etcd-io/bbolt"

type Checkpointer struct {
    db *bolt.DB
}

func NewCheckpointer(path string) (*Checkpointer, error) {
    db, err := bolt.Open(path, 0600, nil)
    if err != nil {
        return nil, err
    }
    // Create a 'bucket' for our files if it doesn't exist
    err = db.Update(func(tx *bolt.Tx) error {
        _, err := tx.CreateBucketIfNotExists([]byte("ProcessedFiles"))
        return err
    })
    return &Checkpointer{db: db}, err
}

func (c *Checkpointer) IsProcessed(filename string) bool {
    var exists bool
    c.db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("ProcessedFiles"))
        v := b.Get([]byte(filename))
        exists = (v != nil)
        return nil
    })
    return exists
}

func (c *Checkpointer) MarkAsDone(filename string) error {
    return c.db.Update(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("ProcessedFiles"))
        return b.Put([]byte(filename), []byte("completed"))
    })
}

```

---

### 3. Integrating with your Pipeline

Update your **Scanner** and **Worker** to use this logic.

#### The Scanner (Skip logic):

```go
filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
    if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".pdf" {
        // ONLY send to workers if we haven't seen it before
        if !checkpointer.IsProcessed(path) {
            jobs <- path
        } else {
            fmt.Printf("Skipping already processed: %s\n", path)
        }
    }
    return nil
})

```

#### The Worker (Success logic):

The worker should only mark the file as done **after** the batch embedding succeeds.

```go
// Inside worker loop...
resp, err := client.Embed(ctx, &api.EmbedRequest{...})
if err == nil {
    // Success! Map results and then...
    checkpointer.MarkAsDone(path) 
}

```

---

### 4. Why `bbolt` is the right tool here

| Feature | Why it matters for your AI Pipeline |
| --- | --- |
| **ACID Compliance** | If the power cuts out mid-write, the database won't corrupt. |
| **Single File** | Your checkpoint "state" is just one file (`checkpoints.db`) you can move with your Parquet file. |
| **Speed** | Checking if a file exists takes microseconds, so it won't slow down your scanner. |

### Final Workflow Summary

1. **Start:** The program opens `checkpoints.db`.
2. **Scan:** It looks at 1,000 PDFs but sees that 900 are already in the DB.
3. **Process:** It only sends the 100 *new* PDFs to Ollama.
4. **Save:** New vectors are appended to your Parquet file, and the DB is updated.

This setup makes your pipeline "resilient." You could even set this up as a **Cron Job** that runs every hour; it will only process new PDFs that you drop into the folder!

##########################################################################################################################




To add a real-time progress bar in Go, the `vbauerster/mpb` (Multi Progress Bar) library is the gold standard. It allows you to display a global progress bar for the entire folder, or even individual bars for each worker.

For this pipeline, we will use a **Global Bar** that increments as each PDF is fully processed.

### 1. The Progress Logic

To implement this, we need to count the total number of files *before* we start the workers so the progress bar knows its "total" capacity.

### 2. Implementation: Integrated Progress Bar

```go
import (
    "github.com/vbauerster/mpb/v8"
    "github.com/vbauerster/mpb/v8/decor"
)

func main() {
    // ... previous setup code ...

    // 1. Pre-scan to get total count for the progress bar
    var files []string
    filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
        if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".pdf" {
            if !checkpointer.IsProcessed(path) {
                files = append(files, path)
            }
        }
        return nil
    })

    // 2. Initialize the Progress Container
    p := mpb.New(mpb.WithWidth(64))
    totalFiles := int64(len(files))
    
    // Add a bar
    bar := p.AddBar(totalFiles,
        mpb.PrependDecorators(
            decor.Name("Processing PDFs: "),
            decor.Percentage(decor.WCSyncSpace),
        ),
        mpb.AppendDecorators(
            decor.OnComplete(decor.AverageETA(decor.ET_STYLE_GO), "done!"),
        ),
    )

    // 3. Start Workers and pass the bar reference to them
    for w := 1; w <= numWorkers; w++ {
        wg.Add(1)
        go embeddingWorker(w, jobs, results, &wg, bar) // pass the bar
    }

    // 4. Send jobs from our pre-scanned list
    go func() {
        for _, path := range files {
            jobs <- path
        }
        close(jobs)
    }()

    // Wait for progress container to finish rendering
    p.Wait()
}

```

### 3. Updating the Worker

Inside the worker, simply call `bar.Increment()` whenever a PDF is finished.

```go
func embeddingWorker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup, bar *mpb.Bar) {
    defer wg.Done()
    // ... extraction & embedding logic ...
    
    // After all chunks for the PDF are sent to results:
    bar.Increment() 
    checkpointer.MarkAsDone(path)
}

```

---

### Why this is essential for "Big Data" pipelines:

1. **ETA Calculation:** The `decor.AverageETA` decorator uses moving averages to tell you exactly how many hours/minutes are left based on current throughput.
2. **Visual Feedback:** It prevents you from wondering if the program has "hung" during long-running embedding tasks.
3. **Error Transparency:** If the bar stops moving, you know exactly which file caused the bottleneck or crash.

### Final Summary of your Pipeline:

* **Go Performance:** Concurrency via Goroutines.
* **ML Ready:** Sliding window chunking + Parquet output.
* **AI Integrated:** Native Ollama embeddings.
* **Production Robust:** Checkpointing with `bbolt`.
* **UX Friendly:** Real-time progress tracking with `mpb`.

####################################################################################################

For this to function as a **training data pipeline** for the **Neural Miner** (which uses SHA-256 ASIC chips for inference as described in `HASHER_SDD.md`), several critical components are missing to bridge the gap between "ingested PDF text" and a "trainable hash-based neural network".

### 1. Tokenization (Text to IDs)

The current pipeline stores raw text strings or high-dimensional float embeddings. The **Neural Miner**'s recursive loop produces **Golden Nonces** that represent specific tokens.

* **What's Missing:** A "Tokenizer" stage (e.g., Tiktoken or BPE) that converts text chunks into a sequence of **integer Token IDs**. The training target for the vHasher will be predicting the "Next Token ID" in the sequence, similar to a standard LLM.

### 2. Feature Mapping (The "Encoder")

The `HASHER_SDD.md` defines the input for the ASIC as a `neural_frame` with 12 slots of 32-bit integers.

* **What's Missing:** Logic that maps your ingested data (like the semantic embeddings from Ollama) into these **12 specialized 32-bit feature slots**. This is essentially your "Encoder" that prepares data for the SHA-256 "Neuron" forward pass.

### 3. Training "Target" Definition

The ingestion engine creates `DocumentRecord` objects but does not define the **labels** for training.

* **What's Missing:** A "Target Generator" that takes your chunked text and creates **(Input, Target)** pairs. For example, if training the model as a knowledge-base assistant, the "Target" would be the ground-truth Token ID following a specific context chunk.

### 4. Weights Database (Seed Persistence)

Your Parquet schema tracks metadata like `FileName` and `ChunkID`. For the **Evo-GRPO** (Evolutionary Group Relative Policy Optimization) training, the Parquet file must also act as your **Weights Database**.

* **What's Missing:** Fields in the Parquet schema to store the **Optimized Seeds** (32-byte SHA-256 keys) discovered during training for specific tokens or contexts.

### 5. Training Checkpointing

The `DataStructuringEngine.md` mentions `bbolt` for checkpointing file progress.

* **What's Missing:** A **Model Checkpointing** system. In evolutionary training, you must save the "State of the Population" (the current best seeds) at intervals to resume training or perform "back-testing" on older model versions.

### Summary: Pipeline Gap Analysis

| Current Ingestion Pipeline | Missing Training Layer | Purpose |
| --- | --- | --- |
| PDF Extraction | **Tokenization** | Convert text to integer IDs for prediction. |
| Ollama Embeddings | **Feature Mapping** | Reshape vectors into the 12-slot `neural_frame`. |
| Parquet Writing | **Seed Persistence** | Store learned 32-byte SHA-256 keys (weights). |
| Local Progress Tracking | **Advantage Calculation** | Implement GRPO reward logic to filter winning seeds. |

**Would you like to see a Go struct definition for a `TrainingRecord` that combines your current PDF data with the specific fields needed for the Neural Miner's training loop?**