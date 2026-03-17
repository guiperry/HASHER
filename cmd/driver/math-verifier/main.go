package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"hasher/pkg/hashing/api"
	"hasher/pkg/hashing/math"
)

var (
	port       = flag.String("port", ":50051", "Port to listen on (e.g. :50051)")
	schemaPath = flag.String("schema", "", "Path to slot-schema YAML file (e.g. pipeline/2_DATA_ENCODER/config/math_schema.yaml)")
	subdomain  = flag.Uint("subdomain", 0, "Math subdomain override: 0=Arithmetic 1=Algebra 2=Calculus 3=Statistics 4=Logic (ignored when --schema is set)")
)

func main() {
	flag.Parse()

	subdomainVal := uint32(*subdomain)
	if subdomainVal > 4 {
		log.Fatalf("Invalid subdomain: %d (must be 0–4)", subdomainVal)
	}

	config := &api.ServerConfig{
		Port:      *port,
		Subdomain: subdomainVal,
	}

	if *schemaPath != "" {
		if _, err := os.Stat(*schemaPath); err != nil {
			log.Fatalf("Schema file not found: %s", *schemaPath)
		}
		config.SchemaPath = *schemaPath
		log.Printf("Schema: %s", *schemaPath)
	} else {
		log.Printf("Schema: (none — using hard-coded fallback rules)")
	}

	log.Printf("Starting MATHASHER Math Verification Server")
	log.Printf("Domain:    MATHASHER")
	log.Printf("Subdomain: %s (%d)", math.GetSubDomainName(subdomainVal), subdomainVal)
	log.Printf("Port:      %s", *port)
	log.Printf("Endpoints: POST /v1/verify/math  |  GET /health")

	go func() {
		if err := api.StartServer(config); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
