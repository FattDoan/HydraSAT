package main

import (
	"log"
	"os"

	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "HydraSAT/proto"
)

func main() {
	// Connect to master gRPC server
	masterAddr := os.Getenv("MASTER_ADDR")
	if masterAddr == "" {
		masterAddr = "localhost:1208"
	}

	conn, err := grpc.NewClient(masterAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to master at %s: %v", masterAddr, err)
	}
	defer conn.Close()

	client := pb.NewSolverServiceClient(conn)

	// Create monitor UI
	ui := NewMonitorUI(client, masterAddr)

	// Run Bubble Tea program (No more imperative options!)
	p := tea.NewProgram(ui)
	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running TUI: %v", err)
	}

}
