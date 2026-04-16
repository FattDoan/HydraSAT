package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const heuristicConfigPath = "heuristic.json"

func main() {
	fmt.Printf("HydraSAT Master Server\n")
	fmt.Printf("========================\n")

	cfg, err := LoadHeuristicConfig(heuristicConfigPath)
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	logger, err := NewResultLogger(cfg.OutputLog)
	if err != nil {
		log.Fatalf("Logger error: %v", err)
	}

	var cnfFiles []string

	if cfg.ReadFolder {
		entries, err := os.ReadDir(cfg.InputFolder)
		if err != nil {
			log.Fatalf("Cannot read input folder %s: %v", cfg.InputFolder, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".cnf") {
				cnfFiles = append(cnfFiles, filepath.Join(cfg.InputFolder, e.Name()))
			}
		}
		if len(cnfFiles) == 0 {
			log.Fatalf("No .cnf files found in %s", cfg.InputFolder)
		}
		fmt.Printf("[main] readFolder: found %d .cnf files in %s\n", len(cnfFiles), cfg.InputFolder)
	} else {
		formulaPath := ""
		if len(os.Args) >= 2 {
			formulaPath = os.Args[1]
		} else if env := os.Getenv("FILE"); env != "" {
			formulaPath = env
		} else {
			log.Fatal("No CNF file provided. Pass as argument, set FILE=, or set readFolder:true in heuristic.json")
		}
		cnfFiles = []string{formulaPath}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	// ── Create the server once and bind the gRPC port once ───────────────────
	// The server lives for the entire process. Workers connect once and stay
	// connected across all CNF files — they never get disconnected between runs.
	server := NewMasterServer(cfg, logger)

	if err := server.Listen(port); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
	fmt.Printf("[main] Workers can now connect. TUI: run './tui_bin' in another terminal.\n\n")

	// ── Solve each file in turn ───────────────────────────────────────────────
	for i, formulaPath := range cnfFiles {
		fmt.Printf("\n[%d/%d] Loading CNF: %s\n", i+1, len(cnfFiles), formulaPath)

		cnfData, err := ParseCNF(formulaPath)
		if err != nil {
			log.Fatalf("Error parsing CNF file %s: %v", formulaPath, err)
		}
		fmt.Printf("  %d vars, %d clauses\n", cnfData.NumVars, cnfData.NumClauses)

		initialCubes := [][]int32{{}} // start with the empty cube
		for i := 0; i < 6; i++ {
			var newCubes [][]int32
			for _, cube := range initialCubes {
				splits := SplitCubeSmart(cube, cnfData.Clauses)
				newCubes = append(newCubes, splits...)
			}
			initialCubes = newCubes
		}

		if err := server.Solve(cnfData, formulaPath, initialCubes); err != nil {
			log.Fatalf("Solve error for %s: %v", formulaPath, err)
		}

		fmt.Println("=======================================")
		fmt.Println("SOLVING COMPLETE")
		fmt.Printf("  File:        %s\n", formulaPath)
		fmt.Printf("  Model Count: %s\n", server.GetTotalCount().Text(10))
		fmt.Println("=======================================")
	}

	// ── All files done — now shut down the gRPC server ───────────────────────
	// Workers will see UNAVAILABLE at this point, which is expected and correct
	// (the whole batch is finished).
	server.Shutdown()
	fmt.Println("\n[main] All done.")
}
