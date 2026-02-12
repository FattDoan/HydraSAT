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

    # Worker Generation
    for i in range(num_workers):
        compose_content += f"""
  worker-{i}:
    build:
      context: .
      dockerfile: python-worker/Dockerfile.worker
    cpuset: "{i}"
    # Use environment variables for cleaner config
    environment:
      - MASTER_ADDR=${{MASTER_IP:-master}}:50051
      - WORKER_ID=worker-{i}
    # Notice we don't strictly need the command args if we use env vars in Python
    command: ["--master", "${{MASTER_IP:-master}}:50051", "--id", "worker-{i}"]
"""

    with open("docker-compose.yaml", "w") as f:
        f.write(compose_content.strip())

if __name__ == "__main__":
    cores = int(sys.argv[1]) if len(sys.argv) > 1 else 4
    generate_yaml(cores)
