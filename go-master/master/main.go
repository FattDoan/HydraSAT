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
		// Folder mode — FILE is ignored entirely
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
		fmt.Printf("readFolder: found %d .cnf files in %s\n", len(cnfFiles), cfg.InputFolder)

	} else {
		// Single file mode — prefer CLI arg, fall back to FILE env var
		formulaPath := ""
		if len(os.Args) >= 2 {
			formulaPath = os.Args[1]
		} else if env := os.Getenv("FILE"); env != "" {
			formulaPath = env
		} else {
			log.Fatal("No CNF file provided. Set FILE= or pass as argument, or set readFolder: true in heuristic.json")
		}
		cnfFiles = []string{formulaPath}
	}

	// ── Solve each file in turn ───────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	for i, formulaPath := range cnfFiles {
		fmt.Printf("\n[%d/%d] Loading CNF: %s\n", i+1, len(cnfFiles), formulaPath)

		cnfData, err := ParseCNF(formulaPath)
		if err != nil {
			log.Fatalf("Error parsing CNF file %s: %v", formulaPath, err)
		}
		fmt.Printf("  %d vars, %d clauses\n", cnfData.NumVars, cnfData.NumClauses)

		server := NewMasterServer(cnfData, formulaPath, cfg, logger)

		// TODO: Replace with actual cube generation from Arjun
	/* 	initialCubes := [][]int32{
			{1, 2, 3}, {1, 2, -3}, {1, -2, 3}, {1, -2, -3},
			{-1, 2, 3}, {-1, 2, -3}, {-1, -2, 3}, {-1, -2, -3},
		} */
		// using SplitCubeSmart to generate initial cubes based on the formula's clauses
		// do it 4 times to get 2^4 = 16 initial cubes, 
		// which is a reasonable starting point for parallelism without overwhelming the queue
		initialCubes := [][]int32{{}} // start with the empty cube
		for i := 0; i < 4; i++ {
			var newCubes [][]int32
			for _, cube := range initialCubes {
				splits := SplitCubeSmart(cube, cnfData.Clauses)
				newCubes = append(newCubes, splits...)
			}
			initialCubes = newCubes
		}


		fmt.Printf("  Loading %d initial cubes\n", len(initialCubes))
		server.LoadInitialTasks(initialCubes)

		fmt.Printf("  Starting gRPC server on port %s\n", port)
		fmt.Printf("  TUI monitor: Run './tui_bin' in another terminal\n\n")

		if err := server.Start(port); err != nil {
			log.Fatalf("Server error: %v", err)
		}

		fmt.Println("\n=======================================")
		fmt.Println("✓ SOLVING COMPLETE")
		fmt.Printf("  File:        %s\n", formulaPath)
		fmt.Printf("  Model Count: %s\n", server.GetTotalCount().Text(10))
		fmt.Println("=======================================")
	}
}
