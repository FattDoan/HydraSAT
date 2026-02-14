#!/bin/bash

# Use Environment Variables passed from Makefile
CORES_TO_USE=${CORES:-4}
MASTER_IP_ADDR=${MASTER_ADDR:-"master:50051"}
PY_PATH=${PYTHON_BIN:-"python3"}

echo "[Hydra-Swarm] Using Python: $PY_PATH"
echo "[Hydra-Swarm] Launching $CORES_TO_USE workers..."

# Find worker.py path
if [ -f "python-worker/worker.py" ]; then
    WORKER_FILE="python-worker/worker.py"
else
    WORKER_FILE="worker.py"
fi

mkdir -p logs

for ((i=0; i<$CORES_TO_USE; i++))
do
    CPU_ID=$i 
    WORKER_NAME="worker-$i"
    echo "  -> Starting $WORKER_NAME on Core $CPU_ID"
    
    # Crucial: Use the $PY_PATH we detected/passed
    taskset -c $CPU_ID $PY_PATH $WORKER_FILE --master $MASTER_IP_ADDR --id $WORKER_NAME > "logs/worker_$i.log" 2>&1 &
done

wait
