.PHONY: proto master tui up down clean help

# --- Configuration ---
PORT      ?= 1208
CORES     ?= $(shell nproc)
FILE      ?= 
MASTER_IP ?= master

# Variables
PYTHON_VENV = ./venv
PIP         = $(PYTHON_VENV)/bin/pip
PYTHON      = $(PYTHON_VENV)/bin/python

# Binary locations
MASTER_BIN  = ./bin/master_bin
MONITOR_BIN = ./bin/tui_bin

# --- Resolve address for workers to connect to the master ---
define get_ip
$(if $(filter master,$(MASTER_IP)),master,\
$(if $(filter host.docker.internal,$(MASTER_IP)),host.docker.internal,$(MASTER_IP)))
endef

# --- Guards ---
check-cores:
ifeq ($(CORES),)
	$(error Please specify CORES (~number of workers). Example: make up CORES=8)
endif

# Detects readFolder from heuristic.json (requires jq, falls back gracefully)
READ_FOLDER := $(shell jq -r '.readFolder // false' heuristic.json 2>/dev/null)

check-file:
ifeq ($(READ_FOLDER),true)
	@echo "[Hydra] readFolder mode — FILE not required"
else ifeq ($(FILE),)
	$(error FILE is required in single-file mode. Usage: make up FILE=problem.cnf  OR set readFolder: true in heuristic.json)
endif

# check-heuristic warns if the config is missing entirely
check-heuristic:
	@test -f heuristic.json || echo "[Hydra] WARNING: heuristic.json not found — using defaults"


x-ganak:
	@chmod +x src/external/ganak-linux-amd64/ganak

# --- Setup VENV and Generate Protobufs ---
proto:
	@echo "[Hydra] Generating Python Protos..."
	@$(PYTHON) -m grpc_tools.protoc -I./proto \
		--python_out=./python-worker \
		--grpc_python_out=./python-worker \
		./proto/solver.proto
	
	@echo "[Hydra] Generating Go Protos for Master..."
	@mkdir -p go-master/proto
	@protoc --proto_path=proto \
		--go_out=go-master/proto --go_opt=paths=source_relative \
		--go-grpc_out=go-master/proto --go-grpc_opt=paths=source_relative \
		proto/solver.proto
	
	@echo "[Hydra] ✓ Proto generation complete"

# -- Install Python dependencies --
pip:
	@echo "[Hydra] Ensuring venv exists and install necessary dependencies..."
	@test -d $(PYTHON_VENV) || python3 -m venv $(PYTHON_VENV)
	@$(PIP) install grpcio grpcio-tools protobuf psutil

# --- Build Targets ---

# Build Master
master: proto
	@echo "[Hydra] Building Go Master binary..."
	@mkdir -p bin
	@cd go-master/master && go build -o ../../$(MASTER_BIN) .
	@chmod +x $(MASTER_BIN)
	@echo "[Hydra] ✓ Master built: $(MASTER_BIN)"

# Build Monitor (TUI)
tui:
	@echo "[Hydra] Building TUI Monitor binary..."
	@mkdir -p bin
	@cd go-master/tui && go build -o ../../$(MONITOR_BIN) .
	@chmod +x $(MONITOR_BIN)
	@echo "[Hydra] ✓ TUI built: $(MONITOR_BIN)"

# Build everything
build: proto master tui
	@echo "[Hydra] ✓ All binaries built successfully"

# --- Docker Targets ---

# [ROOT] Full swarm (Master + Worker Swarm)
up: master check-file check-cores x-ganak check-heuristic 
	@echo "[Hydra] Launching local swarm in Docker..."
	@echo "Cores: $(CORES) | File: $(FILE) | Port: $(PORT)"
	PORT=$(PORT) FILE=$(FILE) CORES=$(CORES) docker compose up --build

# [ROOT] MASTER ONLY 
master-up: master check-file check-heuristic
	@echo "[Hydra] Launching Master Hub in Docker..."
	@echo "File: $(FILE) | Port: $(PORT)"
	PORT=$(PORT) TARGET_FILE=$(FILE) docker compose up --build master

# [ROOT] WORKERS ONLY 
worker-up: check-cores x-ganak
	$(eval IP := $(call get_ip))
	@echo "[Hydra] Launching Worker Swarm in Docker -> $(IP):$(PORT)..."
	PORT=$(PORT) MASTER_ADDR=$(IP):$(PORT) CORES=$(CORES) docker compose up --build --no-deps worker-swarm

# --- [NON-ROOT] Bare Metal Targets ---

# Run Master (bare metal)
run-master: master check-file check-heuristic
	@echo "[Hydra] Starting Master on port $(PORT)..."
	PORT=$(PORT) $(MASTER_BIN) $(FILE)

# Run Monitor (bare metal)
run-tui: tui
	$(eval IP := $(call get_ip))
	@echo "[Hydra] Starting TUI Monitor -> $(IP):$(PORT)..."
	MASTER_ADDR=$(IP):$(PORT) $(MONITOR_BIN)

# Run Workers (bare metal)
noroot-worker-up: check-cores x-ganak pip
	@chmod +x launch_workers.sh
	$(eval IP := $(call get_ip))
	@echo "[Hydra] Starting bare-metal workers (No-Root) -> $(IP):$(PORT)..."
	PORT=$(PORT) CORES=$(CORES) MASTER_ADDR=$(IP):$(PORT) PYTHON_BIN=$(PYTHON) ./launch_workers.sh 

# --- Complete Bare Metal Stack ---

# Run everything bare metal (3 terminals needed)
# Terminal 1: make bare-master FILE=problem.cnf
# Terminal 2: make bare-workers CORES=4
# Terminal 3: make bare-tui
bare-master: run-master

bare-workers: noroot-worker-up

bare-tui: run-tui

# --- Development Targets ---

# Watch and auto-rebuild master
dev-master:
	@echo "[Hydra] Watching master source files..."
	@find go-master/master -name '*.go' | entr -r make run-master FILE=$(FILE)

# Watch and auto-rebuild tui
dev-tui:
	@echo "[Hydra] Watching TUI source files..."
	@find go-master/tui -name '*.go' | entr -r make run-tui

# --- Testing Targets ---

# Quick test with simple CNF
test: master tui
	@echo "[Hydra] Creating test CNF..."
	@mkdir -p test-data
	@echo "p cnf 3 3" > test-data/test.cnf
	@echo "1 2 0" >> test-data/test.cnf
	@echo "-1 3 0" >> test-data/test.cnf
	@echo "-2 -3 0" >> test-data/test.cnf
	@echo "[Hydra] Test CNF created at test-data/test.cnf"
	@echo "[Hydra] Run: make run-master FILE=test-data/test.cnf"

# --- Cleanup ---

down:
	@echo "[Hydra] Stopping Docker containers..."
	docker compose down

noroot-down:
	@echo "[Hydra] Killing local worker processes..."
	@pkill -f "worker.py" || echo "[Hydra] No workers found."

clean:
	@echo "[Hydra] Cleaning build artifacts..."
	rm -rf bin/
	rm -rf src/master/proto/*.go
	rm -f python-worker/*_pb2*
	@echo "[Hydra] Cleaning Docker..."
	docker compose down --rmi all
	@echo "[Hydra] ✓ Cleanup complete"

clean-deps:
	@echo "[Hydra] Cleaning Go dependencies..."
	@cd go-master/master && go clean -modcache
	@cd go-master/tui && go clean -modcache
	rm -rf $(PYTHON_VENV)
	@echo "[Hydra] ✓ Dependencies cleaned"

# --- Information Targets ---

help:
	@echo "HydraSAT Makefile - Distributed Model Counter"
	@echo ""
	@echo "╔════════════════════════════════════════════════════════════════╗"
	@echo "║                     BUILD TARGETS                              ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║  proto          - Generate protobuf files (Python + Go)        ║"
	@echo "║  master         - Build master binary                          ║"
	@echo "║  tui            - Build TUI monitor binary                     ║"
	@echo "║  build          - Build everything (proto + master + monitor)  ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║                    DOCKER TARGETS (ROOT)                       ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║  up             - Full stack (master + workers)                ║"
	@echo "║                   Usage: make up FILE=problem.cnf CORES=8      ║"
	@echo "║  master-up      - Master only                                  ║"
	@echo "║                   Usage: make master-up FILE=problem.cnf       ║"
	@echo "║  worker-up      - Workers only                                 ║"
	@echo "║                   Usage: make worker-up CORES=8                ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║                  BARE METAL TARGETS (NO ROOT)                  ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║  run-master     - Run master (bare metal)                      ║"
	@echo "║                   Usage: make run-master FILE=problem.cnf      ║"
	@echo "║  run-tui    	  - Run TUI  (bare metal)                       ║"
	@echo "║                   Usage: make run-tui                          ║"
	@echo "║  noroot-worker-up - Run workers (bare metal)                   ║"
	@echo "║                   Usage: make noroot-worker-up CORES=4         ║"
	@echo "║                                                                ║"
	@echo "║  3-Terminal Setup:                                             ║"
	@echo "║    Terminal 1: make run-master FILE=problem.cnf                ║"
	@echo "║    Terminal 2: make noroot-worker-up CORES=4                   ║"
	@echo "║    Terminal 3: make run-tui                                    ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║                   DEVELOPMENT TARGETS                          ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║  dev-master     - Watch & auto-rebuild master                  ║"
	@echo "║  dev-tui    - Watch & auto-rebuild TUI                         ║"
	@echo "║  test           - Create test CNF file                         ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║                    CLEANUP TARGETS                             ║"
	@echo "╠════════════════════════════════════════════════════════════════╣"
	@echo "║  down           - Stop Docker containers                       ║"
	@echo "║  noroot-down    - Kill bare metal workers                      ║"
	@echo "║  clean          - Remove build artifacts & Docker images       ║"
	@echo "║  clean-deps     - Remove all dependencies                      ║"
	@echo "╚════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "Configuration:"
	@echo "  PORT=$(PORT)"
	@echo "  CORES=$(CORES)"
	@echo "  FILE=$(FILE)"
	@echo "  MASTER_IP=$(MASTER_IP)"
	@echo ""
	@echo "Examples:"
	@echo "  make build                              # Build all binaries"
	@echo "  make up FILE=problem.cnf CORES=8        # Docker full stack"
	@echo "  make run-master FILE=problem.cnf        # Bare metal master"
	@echo "  make run-tui                            # Bare metal TUI"
	@echo "  make test                               # Create test file"
	@echo ""

status:
	@echo "[Hydra] Current Status:"
	@echo "  Master binary:  $(if $(wildcard $(MASTER_BIN)),✓ Built,✗ Not built)"
	@echo "  Monitor binary: $(if $(wildcard $(MONITOR_BIN)),✓ Built,✗ Not built)"
	@echo "  Master proto:   $(if $(wildcard src/master/proto/*.pb.go),✓ Generated,✗ Not generated)"
	@echo "  Python venv:    $(if $(wildcard $(PYTHON_VENV)),✓ Exists,✗ Not created)"
	@echo ""
	@echo "Configuration:"
	@echo "  PORT:      $(PORT)"
	@echo "  CORES:     $(CORES)"
	@echo "  FILE:      $(FILE)"
	@echo "  MASTER_IP: $(MASTER_IP)"

# Default target
.DEFAULT_GOAL := help
