package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"strconv"
)

// CNFData holds the parsed CNF formula.
// Body is the raw clause text forwarded to workers in TaskPayload.
// Clauses is the parsed representation used server-side for splitting heuristics.
type CNFData struct {
	NumVars    int32
	NumClauses int32
	Body       string      // raw lines, joined by "\n" — sent to workers verbatim
	Clauses    [][]int32   // parsed literals per clause — used by SplitCube
}
 
func ParseCNF(path string) (*CNFData, error) {
	if !strings.HasSuffix(path, ".cnf") {
		return nil, fmt.Errorf("%s is not a .cnf file", path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", path)
	}
 
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
 
	var (
		bodyLines           []string
		clauses             [][]int32
		numVars, numClauses int32
	)
 
	scanner := bufio.NewScanner(f)
	// Some CNF files have very long lines (large formulas)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
 
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "c") {
			continue
		}
		if strings.HasPrefix(line, "p cnf") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				fmt.Sscanf(fields[2], "%d", &numVars)
				fmt.Sscanf(fields[3], "%d", &numClauses)
			}
			continue
		}
 
		// Clause line — parse into []int32 and keep raw text
		bodyLines = append(bodyLines, line)
 
		fields := strings.Fields(line)
		var clause []int32
		for _, tok := range fields {
			v, err := strconv.ParseInt(tok, 10, 32)
			if err != nil {
				continue
			}
			if v == 0 {
				break // end-of-clause sentinel
			}
			clause = append(clause, int32(v))
		}
		if len(clause) > 0 {
			clauses = append(clauses, clause)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
 
	return &CNFData{
		NumVars:    numVars,
		NumClauses: numClauses,
		Body:       strings.Join(bodyLines, "\n"),
		Clauses:    clauses,
	}, nil
}
