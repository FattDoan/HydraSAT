import sys

def generate_yaml(num_workers):
    # Base Master Configuration
    # We use ${TARGET_FILE} so the Master knows which problem to solve
    compose_content = f"""
services:
  master:
    build:
      context: .
      dockerfile: go-master/Dockerfile.master
    ports:
      - "50051:50051"
    command: ["/data/${{TARGET_FILE}}"]
    volumes:
      - ./cnf_instances:/data
"""

    # Worker Generation with Strict Core Pinning
    for i in range(num_workers):
        compose_content += f"""
  worker-{i}:
    build:
      context: .
      dockerfile: python-worker/Dockerfile.worker
    # cpu-pinning: Ensures this worker stays on its own core
    cpuset: "{i}"
    # We pass the ID and Master address directly to the Python script
    command: ["--master", "${{MASTER_ADDR:-master:50051}}", "--id", "worker-{i}"]
"""

    with open("docker-compose.yaml", "w") as f:
        f.write(compose_content.strip())

if __name__ == "__main__":
    cores = int(sys.argv[1]) if len(sys.argv) > 1 else 4
    generate_yaml(cores)
