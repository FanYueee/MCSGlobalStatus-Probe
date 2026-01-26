package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcsapi/probe/internal/client"
)

func main() {
	serverURL := flag.String("server", "ws://localhost:3000", "Controller WebSocket URL")
	nodeID := flag.String("id", "local-01", "Node ID")
	region := flag.String("region", "Local", "Node region")
	secret := flag.String("secret", "", "Probe secret token")
	flag.Parse()

	// Get secret from environment if not provided
	if *secret == "" {
		*secret = os.Getenv("PROBE_SECRET")
		if *secret == "" {
			*secret = "default-secret"
		}
	}

	log.Printf("Starting MCSAPI Probe")
	log.Printf("  Node ID: %s", *nodeID)
	log.Printf("  Region: %s", *region)
	log.Printf("  Server: %s", *serverURL)

	c := client.NewClient(*serverURL, *nodeID, *region, *secret)

	if err := c.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		c.Close()
		os.Exit(0)
	}()

	// Run the client
	c.Run()
}
