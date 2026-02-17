.PHONY: proto master up down clean

# --- Configuration ---
PORT 	  ?= 1208
CORES     ?= $(shell nproc)
FILE      ?= 
MASTER_IP ?= master


# Variables
PYTHON_VENV = ./venv
PIP         = $(PYTHON_VENV)/bin/pip
PYTHON      = $(PYTHON_VENV)/bin/python

# --- Resolve address for workers to connect to the master ---
# If MASTER_IP is "master", return master:PORT
# If it's "host.docker.internal", return host.docker.internal:PORT
# If it has a colon already (ngrok 0.tcp.eu.ngrok.io:12345), use as-is.
define get_addr
$(if $(findstring :,$(MASTER_IP)),$(MASTER_IP),\
$(if $(filter master,$(MASTER_IP)),master:$(PORT),\
$(if $(filter host.docker.internal,$(MASTER_IP)),host.docker.internal:$(PORT),$(MASTER_IP):$(PORT))))
endef

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
	@echo "[Hydra] Launching local swarm in Docker..."
	@echo "Nb of cores: $(CORES) | File: $(FILE) | Port: $(PORT)"
	PORT=$(PORT) FILE=$(FILE) CORES=$(CORES) docker compose up --build

# [ROOT] MASTER ONLY 
# Usage: make master-up FILE=problem.cnf
master-up: master check-file
	@echo "[Hydra] Launching Master Hub in Docker..."
	@echo "File: $(FILE) | Port: $(PORT)"
	PORT=$(PORT) ARGET_FILE=$(FILE) docker compose up --build master

# [ROOT] WORKERS ONLY 
worker-up: x-ganak
	$(eval ADDR := $(call get_addr))
	@echo "[Hydra] Launching Worker Swarm in Docker -> $(ADDR)..."
	PORT=$(PORT) MASTER_ADDR=$(ADDR) CORES=$(CORES) docker compose up --build --no-deps worker-swarm

# --- [NON-ROOT] Bare Metal Targets ---
noroot-worker-up: check-cores x-ganak
	@chmod +x launch_workers.sh
	$(eval ADDR := $(call get_addr))
	@# If still "master:PORT" but on bare metal, fix to localhost
	$(eval FINAL_ADDR := $(subst master:,localhost:,$(ADDR)))
	@echo "[Hydra] Starting bare-metal workers (No-Root) -> $(FINAL_ADDR)..."
	PORT=$(PORT) CORES=$(CORES) MASTER_ADDR=$(FINAL_ADDR) PYTHON_BIN=$(PYTHON) ./launch_workers.sh 


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
