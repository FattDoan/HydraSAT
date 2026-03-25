package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type CNFData struct {
	NumVars    int32
	NumClauses int32
	Body       string
}

func ParseCNF(path string) (*CNFData, error) {
	if !strings.HasSuffix(path, ".cnf") {
		return nil, fmt.Errorf("file %s is not a .cnf file", path)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file %s does not exist", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var body []string
	var numVars, numClauses int32

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "c") {
			continue
		}

		// Parse header
		if strings.HasPrefix(line, "p cnf") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				fmt.Sscanf(fields[2], "%d", &numVars)
				fmt.Sscanf(fields[3], "%d", &numClauses)
			}
			continue
		}

		// Clause line
		body = append(body, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &CNFData{
		NumVars:    numVars,
		NumClauses: numClauses,
		Body:       strings.Join(body, "\n"),
	}, nil
}
