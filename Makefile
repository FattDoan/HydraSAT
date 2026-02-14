.PHONY: proto master up down clean

# --- Configuration ---
CORES     ?= $(shell nproc)
FILE      ?= 
MASTER_IP ?= master
# MASTER_ADDR combines IP and the gRPC port
MASTER_ADDR = $(MASTER_IP):50051

# Variables
PYTHON_VENV = ./venv
PIP         = $(PYTHON_VENV)/bin/pip
PYTHON      = $(PYTHON_VENV)/bin/python

# --- Guards ---
check-cores:
ifeq ($(CORES),)
	$(error Please specify CORES (~number of workers). Example: make up CORES=8)
endif

check-file:
ifeq ($(FILE),)
	$(error Please specify cnf FILE path. Example: make up FILE=problem.cnf)
endif

x-ganak:
	@chmod +x src/external/ganak-linux-amd64/ganak

# --- Setup VENV and Generate Protobufs ---
proto:
	@echo "[Hydra] Generating Python Protos..."
	@$(PYTHON) -m grpc_tools.protoc -I./proto \
		--python_out=./python-worker \
		--grpc_python_out=./python-worker \
		./proto/solver.proto
	
	@echo "[Hydra] Generating Go Protos..."
	@mkdir -p go-master/proto
	@protoc --proto_path=proto \
		--go_out=go-master/proto --go_opt=paths=source_relative \
		--go-grpc_out=go-master/proto --go-grpc_opt=paths=source_relative \
		proto/solver.proto

# -- If only need to launch workers (on non-root), then dont need to make-proto --
#  -- just make pip --
pip:
	@echo "[Hydra] Ensuring venv exists and install necessary dependencies..."
	@test -d $(PYTHON_VENV) || python3 -m venv $(PYTHON_VENV)
	# Install both the tools and the library itself
	@$(PIP) install grpcio grpcio-tools protobuf


master:
	@echo "[Hydra] Building Go Master binary..."
	@cd go-master && go build -o ../master_bin main.go


# -- [ROOT] Docker Targets ---
# [ROOT] Full swarm (Master + Worker Swarm)
up: master check-file x-ganak 
	@echo "[Hydra] Launching local swarm in Docker (Cores: $(CORES)) (File: $(FILE))..."
	FILE=$(FILE) CORES=$(CORES) docker compose up --build

# [ROOT] MASTER ONLY 
# Usage: make master-up FILE=problem.cnf
master-up: master check-file
	@echo "[Hydra] Launching Master Hub in Docker..."
	TARGET_FILE=$(FILE) docker compose up --build master

# [ROOT] WORKERS ONLY 
worker-up: x-ganak
	@echo "[Hydra] Launching Worker Swarm in Docker (Target: $(MASTER_ADDR))..."
	MASTER_ADDR=$(MASTER_ADDR) CORES=$(CORES) docker compose up --build --no-deps worker-swarm

# --- [NON-ROOT] Bare Metal Targets ---
noroot-worker-up: check-cores x-ganak
	@chmod +x launch_workers.sh
	@echo "[Hydra] Starting bare-metal workers (No-Root)..."
	# If MASTER_IP is still "master", we switch it to "127.0.0.1" for local mode
	$(eval ADDR := $(if $(filter master,$(MASTER_IP)),127.0.0.1,$(MASTER_IP)))
	CORES=$(CORES) MASTER_ADDR=$(ADDR):50051 PYTHON_BIN=$(PYTHON) ./launch_workers.sh 


# --- Cleanup ---
down:
	docker compose down

noroot-down:
	@echo "[Hydra] Killing local worker processes..."
	@pkill -f "worker.py" || echo "[Hydra] No workers found."

clean:
	rm -f master_bin
	rm -rf go-master/proto/*.go
	rm -f python-worker/*_pb2*
	docker compose down --rmi all
