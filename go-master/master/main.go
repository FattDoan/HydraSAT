package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	// Check if a file path was passed as an argument
	if len(os.Args) < 2 {
		log.Fatal("Error: No CNF file provided. Usage: ./master_bin problem.cnf")
	}
	formulaPath := os.Args[1]

	fmt.Printf("HydraSAT Master Server\n")
	fmt.Printf("========================\n")
	fmt.Printf("Loading CNF from: %s\n", formulaPath)

	// Parse CNF file once
	cnfData, err := ParseCNF(formulaPath)
	if err != nil {
		log.Fatalf("Error parsing CNF file: %v", err)
	}

	fmt.Printf("CNF loaded: %d vars, %d clauses\n", cnfData.NumVars, cnfData.NumClauses)

	// Initialize master server
	server := NewMasterServer(cnfData)

	// Generate initial cubes
	// TODO: Replace with actual cube generation from Arjun
	initialCubes := [][]int32{
		{1, 2, 3}, {1, 2, -3}, {1, -2, 3}, {1, -2, -3},
		{-1, 2, 3}, {-1, 2, -3}, {-1, -2, 3}, {-1, -2, -3},
	}

	fmt.Printf("Loading %d initial cubes into queue\n", len(initialCubes))
	server.LoadInitialTasks(initialCubes)

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	fmt.Printf("Starting gRPC server on port %s\n", port)
	fmt.Printf("TUI monitor: Run './monitor' in another terminal\n")
	fmt.Printf("Press Ctrl+C to stop\n\n")

	// Start server (blocks until done)
	if err := server.Start(port); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	// Print final results
	fmt.Println("\n=======================================")
	fmt.Println("✓ SOLVING COMPLETE")
	fmt.Printf("Final Model Count: %s\n", server.GetTotalCount().Text(10))
	fmt.Println("=======================================")
}
