// Command ics is the ICS-Core binary: it runs the Message Bus HTTP server
// and the AI Pipeline consumer together in a single process.
package main

import (
	"log"

	"ics/bus"
	"ics/pipeline"
)

func main() {
	cfg := bus.LoadConfig()

	// Pipeline is a passive SSE consumer; it authenticates to the bus with the
	// same token and runs alongside the server.
	go pipeline.Start(cfg.Token)

	if err := bus.Run(cfg); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
