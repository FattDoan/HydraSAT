package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	pb "HydraSAT/proto"
)

// Task represents a subproblem (a cube)
type Task struct {
	Literals []int32
}

type masterServer struct {
	pb.UnimplementedSolverServiceServer
	taskQueue chan *pb.CountRequest

	checkbook      map[int64][]int32
	checkbookMutex sync.Mutex
	nextID         int64

	totalCount *big.Int
	countMutex sync.Mutex

	activeTasks int32
	cnfData     *CNFData
	done        chan bool
}

/* // Global counter to track the #SAT count
var (
	totalCount  = big.NewInt(0) // init BigInt
	countMutex  sync.Mutex      // mutex to protect totalCount updates
	activeTasks int32           // tracks pending cubes in the system
) */

// Register is called by workers to receive tasks (cubes) to process
func (s *masterServer) GetTask(ctx context.Context, req *pb.RegisterRequest) (*pb.CountRequest, error) {
	fmt.Printf("Worker [%s] demanded for task.\n", req.WorkerId)

	select {
	case task := <-s.taskQueue:
		// Task found -> return it to the worker immediately.
		fmt.Printf("Task %d : |%v| assigned to worker %s\n", task.TaskId, task.Literals, req.WorkerId)
		return task, nil

	case <-time.After(5 * time.Second):
		// No tasks in queue. Return an empty task with ID -1
		// to tell the worker to "wait and try again."
		return &pb.CountRequest{TaskId: -1}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// bundle the lits (cube) into a CountRequest and record it in the checkbook with a unique ID
func (s *masterServer) makeRequest(cube []int32) *pb.CountRequest {
	s.checkbookMutex.Lock()
	defer s.checkbookMutex.Unlock()

	id := s.nextID
	s.nextID++

	// Record the literals in our checkbook
	s.checkbook[id] = cube

	return &pb.CountRequest{
		TaskId:      id,
		NumVars:     s.cnfData.NumVars,
		NumClauses:  s.cnfData.NumClauses,
		FormulaBody: s.cnfData.Body,
		Literals:    cube,
		TimeoutSec:  30, // TODO: Tune this dynamically based on cube size or worker performance
	}
}

// SubmitResult is called by workers to submit their count results for a cube
func (s *masterServer) SubmitResult(ctx context.Context, req *pb.CountResponse) (*pb.Empty, error) {
	s.checkbookMutex.Lock()
	cube, exists := s.checkbook[req.TaskId] // Get literals for this task ID
	delete(s.checkbook, req.TaskId)         // Remove from checkbook for done task
	s.checkbookMutex.Unlock()

	if !exists {
		return nil, fmt.Errorf("Received result for unknown task ID %d. Ignoring.\n", req.TaskId)
	}

	if req.TimedOut {
		fmt.Printf("Worker %s timeout for cube %v. Splitting\n", req.WorkerId, cube)
		newCubes := splitCube(cube)
		// Increment active tasks for new cubes, decrement for the one that timed out
		atomic.AddInt32(&s.activeTasks, int32(len(newCubes)-1))

		// add new cubes to the queue for processing
		for _, newTask := range newCubes {
			s.taskQueue <- s.makeRequest(newTask.Literals)
		}

	} else {
		// -- BIG INT HANDLING --
		val := new(big.Int)
		// Parse the count from the response into a big.Int
		val, ok := val.SetString(req.Count, 10)
		if !ok {
			fmt.Printf("Worker %s failed to parse count '%s' for cube %v\n", req.WorkerId, req.Count, cube)
			atomic.AddInt32(&s.activeTasks, -1) // Decrement active tasks on parse error
		} else {
			s.countMutex.Lock()
			s.totalCount.Add(s.totalCount, val)
			s.countMutex.Unlock()

			fmt.Printf("Worker %s finished cube %v with count %s\n", req.WorkerId, cube, val.Text(10))
		}
		newVal := atomic.AddInt32(&s.activeTasks, -1) // Decrement active tasks on successful completion

		fmt.Printf("[REMAINING] Task %d done. Tasks left: %d\n", req.TaskId, newVal)
	}

	// Final completion signal
	if atomic.LoadInt32(&s.activeTasks) == 0 {
		select {
		case s.done <- true: // Signal main thread to finish
		default:
		}
	}

	return &pb.Empty{}, nil
}

func main() {
	// Check if a file path was passed as an argument
	if len(os.Args) < 2 {
		log.Fatal("Error: No CNF file provided. Usage: ./master_bin problem.cnf")
	}
	formulaPath := os.Args[1]

	fmt.Printf("Loading CNF from: %s\n", formulaPath)

	// Parse CNF file once
	cnfData, err := parseCNF(formulaPath)
	if err != nil {
		fmt.Printf("Error parsing CNF file: %v\n", err)
		return
	}




	// Initialize master server state
	m := &masterServer{
		taskQueue:  make(chan *pb.CountRequest, 1000),
		checkbook:  make(map[int64][]int32),
		totalCount: big.NewInt(0),
		done:       make(chan bool),
		cnfData:    cnfData,
	}

	// Load init tasks (cubes) into the task queue
	// TODO: Replace with actual cube gen from C++ worker
	initialCubes := [][]int32{{1, 2, 3}, {1, 2, -3}, {1, -2, 3}, {1, -2, -3},
		{-1, 2, 3}, {-1, 2, -3}, {-1, -2, 3}, {-1, -2, -3}}
	m.activeTasks = int32(len(initialCubes))

	for _, c := range initialCubes {
		// use makeRequest to register them in the checkbook automatically
		m.taskQueue <- m.makeRequest(c)
	}

	// Start gRPC server in a separate goroutine
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		return
	}
	server := grpc.NewServer()
	pb.RegisterSolverServiceServer(server, m)

	fmt.Println("Master listening on :50051 via Tailscale/Local...")
	go func() {
		if err := server.Serve(lis); err != nil {
			fmt.Printf("Server failed: %v\n", err)
		}
	}()
	// block until done signal is received (all tasks completed)
	<-m.done

	fmt.Println("---------------------------------------")
	fmt.Printf("SOLVING COMPLETE\n")
	fmt.Printf("Final Model Count: %s\n", m.totalCount.Text(10))
	fmt.Println("---------------------------------------")

	server.GracefulStop()
}

// TODO: implement more sophisticated splitting heuristics
// Can change this func to send RPC back to a C++ worker to do the split (FlowCutter)
func splitCube(cube []int32) []Task {
	// basic split: find the max variable and add the next one
	var maxVar int32 = 0
	for _, lit := range cube {
		abs := lit
		if abs < 0 {
			abs = -lit
		}
		if abs > maxVar {
			maxVar = abs
		}
	}
	nextVar := maxVar + 1

	return []Task{
		{Literals: append(append([]int32{}, cube...), nextVar)},
		{Literals: append(append([]int32{}, cube...), -nextVar)},
	}
}

type CNFData struct {
	NumVars    int32
	NumClauses int32
	Body       string
}

func parseCNF(path string) (*CNFData, error) {
	if !strings.HasSuffix(path, ".cnf") {
		return nil, fmt.Errorf("[Master]: File %s is not a .cnf file", path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
    	return nil, fmt.Errorf("[Master]: File %s does not exist!", path)
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

		if strings.HasPrefix(line, "p cnf") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				fmt.Sscanf(fields[2], "%d", &numVars)
				fmt.Sscanf(fields[3], "%d", &numClauses)
			}
			continue
		}

		body = append(body, line)
	}

	return &CNFData{
		NumVars:    numVars,
		NumClauses: numClauses,
		Body:       strings.Join(body, "\n"),
	}, nil
}
