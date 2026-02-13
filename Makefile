.PHONY: proto master up down clean

# Default value if want one, or leave empty to force input
CORES ?=
FILE ?=
# If use tailscale, set this to the given IP of the master machine
MASTER_IP ?= master #Default to internal docker networking


# Variables
PYTHON_VENV = ./venv
PIP = $(PYTHON_VENV)/bin/pip
PYTHON = $(PYTHON_VENV)/bin/python


# Helper to check for CORES variable
check-cores:
ifeq ($(CORES),)
	$(error Please specify CORES (~number of workers). Example: make up CORES=8)
endif

check-file:
ifeq ($(FILE),)
	$(error Please specify CNF FILE in cnf_instances folder. Example: make up FILE=problem.cnf)
endif

# Setup VENV and Generate Protobufs
proto:
	@echo "[Hydra] Ensuring venv exists..."
	@test -d $(PYTHON_VENV) || (python3 -m venv $(PYTHON_VENV) && $(PIP) install grpcio-tools)
	
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

# Build Go Master locally (for quick debugging)
master:
	@echo "[Hydra] Building Go Master binary..."
	@cd go-master && go build -o ../master_bin main.go

# Docker Orchestration
gen-yaml: check-cores
	@echo "[Hydra] Generating config for $(CORES) cores..."
	@python3 gen_compose.py $(CORES)

# LOCAL ALL IN ONE: master + workers on same machine
up: gen-yaml check-file master
	@echo "[Hydra] Launching local swarm..."
	TARGET_FILE=$(FILE) docker compose up --build --abort-on-container-exit

# Launch ONLY the Master
# Usage: make master-up FILE=problem.cnf
master-up: master check-file
	@echo "[Hydra] Launching Master Hub..."
	TARGET_FILE=$(FILE) docker compose up --build master

# Launch ONLY Workers (connects to Master via environment variable)
# Usage: make worker-up CORES=4 MASTER_IP=100.x.y.z
worker-up: check-cores
	MASTER_IP=$(MASTER_IP) python3 gen_compose.py $(CORES)
	@echo "[Hydra] Launching $(CORES) workers connecting to $(MASTER_IP)..."
	MASTER_IP=$(MASTER_IP) docker compose up --build $(shell grep -o "worker-[0-9]*" docker-compose.yaml | xargs) 
clean:
	rm -f master_bin
	rm -rf go-master/proto/*.go
	rm -f python-worker/*_pb2*
	docker compose down --rmi all
