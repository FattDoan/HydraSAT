#pragma once

#include <atomic>
#include <condition_variable>
#include <mutex>
#include <queue>
#include <thread>
#include <vector>

#include "3rdParty/d4/3rdParty/bipe/src/reducer/Propagator.hpp"
#include "src/branchingHeuristic/D4BranchingCubeHeuristic.hpp"
#include "partialOrderHeuristic/PartialOrderHeuristicTreeDecompLocal.hpp"
#include "src/formula/Formula.hpp"
#include "src/formula/ProblemTypes.hpp"

namespace discount {

struct Cube {
  std::vector<Lit> lits;
  double weight;
};

using PriorityQueue =
    std::priority_queue<Cube, std::vector<Cube>, bool (*)(Cube, Cube)>;

struct WorkerResult {
  std::vector<Lit> lits;
  bool sat;
};

class CubeGeneratorCLI {
 public:
  explicit CubeGeneratorCLI(unsigned nbThreads = 4);
  ~CubeGeneratorCLI();

  bool generate(Formula* formula,
                std::vector<std::vector<Lit>>& cubes,
                unsigned limitNbCubes,
                double timeBudgetSeconds);

  void getPartialOrderComputed(std::vector<unsigned>& partialOrder) const;
  int  getTreewidth() const { return m_treewidth; }

 private:
  void initMainPropagator(Formula* formula);
  void initQueue(PriorityQueue& queue);
  void createCubes(std::vector<std::vector<Lit>>& done,
                   PriorityQueue& queue,
                   std::vector<std::vector<Lit>>& pending);
  void submitSATCheck(std::vector<Lit> cube);
  void collectResults(PriorityQueue& queue, bool block);
  void workerLoop(unsigned id);

  unsigned m_nbThreads;
  unsigned m_nbVariable = 0;
  int      m_treewidth  = -1;

  // main-thread propagator (single-threaded, used only in createCubes)
  bipe::reducer::Propagator*             m_propagator            = nullptr;
  D4BranchingCubeHeuristic*              m_branchingHeuristic    = nullptr;
  PartialOrderHeuristicTreeDecompLocal*  m_partialOrderHeuristic = nullptr;

  // per-thread propagators for SAT feasibility checks
  std::vector<bipe::reducer::Propagator*> m_threadPropagators;
  std::vector<std::vector<bipe::Lit>>     m_bipeClauses;  // kept to init thread propagators

  // thread pool
  std::vector<std::thread>         m_threads;
  std::mutex                       m_queueMtx;
  std::condition_variable          m_queueCV;
  std::queue<std::vector<Lit>>     m_jobQueue;
  bool                             m_shutdown = false;

  std::mutex                       m_resultMtx;
  std::condition_variable          m_resultCV;
  std::vector<WorkerResult>        m_results;
  std::atomic<int>                 m_inFlight{0};
};

}  // namespace discount
