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
	port      = flag.String("port", ":50051", "Port to listen on")
	subdomain = flag.Uint("subdomain", 0, "Math subdomain (0=Arithmetic, 1=Algebra, 2=Calculus, 3=Statistics, 4=Logic)")
)

func main() {
	flag.Parse()

	subdomainVal := uint32(*subdomain)
	if subdomainVal > 4 {
		log.Fatalf("Invalid subdomain: %d (must be 0-4)", subdomainVal)
	}

	config := &api.ServerConfig{
		Port:      *port,
		Subdomain: subdomainVal,
	}

	log.Printf("Starting MATHASHER Math Verification Server")
	log.Printf("Domain: MATHASHER")
	log.Printf("Subdomain: %s (%d)", math.GetSubDomainName(subdomainVal), subdomainVal)
	log.Printf("Port: %s", *port)

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
