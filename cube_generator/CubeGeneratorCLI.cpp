/*
 * CubeGeneratorCLI.cpp
 * Logic is almost copy-pasted from the original CubeGenerator.cpp.
 * Only changes:
 *   • MPI_{Send,Recv,Isend,Irecv,Test,Wait,…} → thread-pool calls
 *   • OptionCubeGenerator removed (VSADS + tree-decomp hardwired)
 *   • collectTerminatedTask / assignTasksToWorkers collapsed into
 *     submitSATCheck / collectResults
 */

#include "CubeGeneratorCLI.hpp"

#include <cassert>
#include <chrono>
#include <iostream>

namespace discount {

// ── constructor / destructor ─────────────────────────────────────────────────

CubeGeneratorCLI::CubeGeneratorCLI(unsigned nbThreads)
    : m_nbThreads(nbThreads) {}

CubeGeneratorCLI::~CubeGeneratorCLI() {
  // shut down thread pool
  {
    std::lock_guard<std::mutex> lk(m_queueMtx);
    m_shutdown = true;
  }
  m_queueCV.notify_all();
  for (auto& t : m_threads) t.join();

  for (auto* s : m_solvers) delete s;
  delete m_propagator;
  delete m_branchingHeuristic;
  delete m_partialOrderHeuristic;
}

// ── thread-pool worker ───────────────────────────────────────────────────────

void CubeGeneratorCLI::workerLoop(unsigned id) {
  d4::WrapperMinisat* solver = m_solvers[id];

  while (true) {
    std::vector<Lit> cube;
    {
      std::unique_lock<std::mutex> lk(m_queueMtx);
      m_queueCV.wait(lk, [this] { return m_shutdown || !m_jobQueue.empty(); });
      if (m_shutdown && m_jobQueue.empty()) return;
      cube = std::move(m_jobQueue.front());
      m_jobQueue.pop();
    }

    // Check SAT feasibility: assume all literals in cube and call solve.
    std::vector<d4::Lit> assumptions;
    assumptions.reserve(cube.size());
    for (auto& l : cube)
      assumptions.push_back(d4::Lit::makeLit(l.var(), l.sign()));

    bool sat = solver->solve(assumptions);

    {
      std::lock_guard<std::mutex> lk(m_resultMtx);
      m_results.push_back({std::move(cube), sat});
    }
    --m_inFlight;
    m_resultCV.notify_one();
  }
}

// ── initPropagator ── (copy-pasted verbatim) ─────────────────────────────────

void CubeGeneratorCLI::initPropagator(Formula* formula) {
  std::vector<std::vector<bipe::Lit>> clauses;
  for (auto& cl : formula->getClauses()) {
    clauses.push_back({});
    for (auto& l : cl)
      clauses.back().push_back(bipe::Lit::makeLit(l.var(), l.sign()));
  }
  m_propagator =
      new bipe::reducer::Propagator(formula->getNbVar(), clauses, false);
}

// ── initQueue ── (copy-pasted verbatim) ──────────────────────────────────────

void CubeGeneratorCLI::initQueue(PriorityQueue& queue) {
  std::vector<Lit> units;
  m_propagator->propagate();
  bipe::Lit* trail = m_propagator->getTrail();
  for (unsigned i = 0; i < m_propagator->getTrailSize(); i++)
    units.push_back(Lit::makeLit(trail[i].var(), trail[i].sign()));
  queue.push({units, (double)units.size()});
}

// ── createCubes ── (copy-pasted, worker dispatch replaced) ───────────────────

void CubeGeneratorCLI::createCubes(std::vector<std::vector<Lit>>& done,
                                    PriorityQueue& queue,
                                    std::vector<std::vector<Lit>>& pending) {
  const Cube& cBest = queue.top();

  // fill propagator with the current cube's literals
  m_propagator->restart();
  for (auto& l : cBest.lits) {
    bipe::Lit lb = bipe::Lit::makeLit(l.var(), l.sign());
    if (m_propagator->value(lb) > 1) m_propagator->uncheckedEnqueue(lb);
  }
  assert(m_propagator->propagate() && !m_propagator->getIsUnsat());
  assert(cBest.lits.size() == m_propagator->getTrailSize());

  // select branching variable (VSADS + partial order from tree decomp)
  Var v = m_branchingHeuristic->next(cBest.lits, m_partialOrderHeuristic);

  if (v == var_Undef) {
    // cube is fully determined → leaf, goes straight to done
    done.push_back(cBest.lits);
    queue.pop();
    return;
  }

  bipe::Lit* trail     = m_propagator->getTrail();
  int        sizeTrail = m_propagator->getTrailSize();

  // branch v = true
  m_propagator->uncheckedEnqueue(bipe::Lit::makeLitTrue(v));
  if (m_propagator->propagate()) {
    std::vector<Lit> branch(trail, trail + m_propagator->getTrailSize());
    // convert bipe::Lit → discount::Lit for positions beyond the base
    for (unsigned i = sizeTrail; i < (unsigned)branch.size(); ++i)
      branch[i] = Lit::makeLit(trail[i].var(), trail[i].sign());
    pending.push_back(std::move(branch));
  }

  m_propagator->cancelUntilPos(sizeTrail);

  // branch v = false
  m_propagator->uncheckedEnqueue(bipe::Lit::makeLitFalse(v));
  if (m_propagator->propagate()) {
    std::vector<Lit> branch;
    branch.reserve(m_propagator->getTrailSize());
    for (unsigned i = 0; i < (unsigned)m_propagator->getTrailSize(); ++i)
      branch.push_back(Lit::makeLit(trail[i].var(), trail[i].sign()));
    pending.push_back(std::move(branch));
  }

  queue.pop();
}

// ── submitSATCheck ───────────────────────────────────────────────────────────

void CubeGeneratorCLI::submitSATCheck(std::vector<Lit> cube) {
  ++m_inFlight;
  {
    std::lock_guard<std::mutex> lk(m_queueMtx);
    m_jobQueue.push(std::move(cube));
  }
  m_queueCV.notify_one();
}

// ── collectResults ───────────────────────────────────────────────────────────

void CubeGeneratorCLI::collectResults(PriorityQueue& queue, bool block) {
  std::unique_lock<std::mutex> lk(m_resultMtx);

  auto hasResults = [this] { return !m_results.empty(); };

  if (block)
    m_resultCV.wait(lk, hasResults);
  else if (!hasResults())
    return;

  for (auto& r : m_results) {
    if (r.sat)
      queue.push({r.lits, (double)r.lits.size()});
    // UNSAT cubes are simply dropped
  }
  m_results.clear();
}

// ── generate ── (main loop; structure mirrors original generate()) ────────────

bool CubeGeneratorCLI::generate(Formula* formula,
                                 std::vector<std::vector<Lit>>& cubes,
                                 unsigned limitNbCubes,
                                 double timeBudgetSeconds) {
  m_nbVariable = formula->getNbVar();

  // ── build heuristics ──
  // VSADS branching (best per paper)
  m_branchingHeuristic = new D4BranchingCubeHeuristic(
      formula, OptionCubeGenerator::BranchingCubeHeuristic::VSADS);

  // Tree-decomposition partial order (best per paper).
  // Runs FlowCutter locally with m_nbThreads seeds; picks best treewidth.
  m_partialOrderHeuristic =
      new PartialOrderHeuristicTreeDecompLocal(formula, m_nbThreads);
  m_treewidth = m_partialOrderHeuristic->getTreewidth();

  // ── spin up thread pool with one solver per thread ──
  m_solvers.resize(m_nbThreads, nullptr);
  for (unsigned i = 0; i < m_nbThreads; ++i) {
    m_solvers[i] = new d4::WrapperMinisat();
    m_solvers[i]->initSolver(*m_branchingHeuristic->getProblemManager());
  }
  for (unsigned i = 0; i < m_nbThreads; ++i)
    m_threads.emplace_back(&CubeGeneratorCLI::workerLoop, this, i);

  // ── propagator + queue ──
  initPropagator(formula);
  PriorityQueue queue([](Cube c1, Cube c2) { return c1.weight > c2.weight; });
  initQueue(queue);

  auto tStart   = std::chrono::steady_clock::now();
  auto deadline = tStart + std::chrono::duration<double>(timeBudgetSeconds);
  double logAt  = 10.0;
  bool unsat    = false;

  // ── main loop (mirrors original while loop) ──
  while (true) {
    bool timeUp = std::chrono::steady_clock::now() >= deadline;

    // expand the best cube if we still need more and nothing is pending
    if (!timeUp && queue.size() < limitNbCubes && m_inFlight == 0 &&
        !queue.empty()) {
      std::vector<std::vector<Lit>> pending;
      createCubes(cubes, queue, pending);
      for (auto& p : pending) submitSATCheck(std::move(p));
    }

    // collect results (non-blocking unless everything is in-flight and queue empty)
    bool mustBlock = queue.empty() && m_inFlight > 0;
    collectResults(queue, mustBlock);

    // logging
    double elapsed = std::chrono::duration<double>(
                         std::chrono::steady_clock::now() - tStart)
                         .count();
    if (elapsed > logAt) {
      std::cerr << "c Elapsed: " << elapsed << "s  cubes: " << queue.size()
                << "/" << limitNbCubes << "\n";
      logAt = elapsed + 10.0;
    }

    // termination conditions
    if (queue.empty() && m_inFlight == 0) {
      unsat = cubes.empty();  // nothing was ever SAT → UNSAT
      break;
    }
    if (queue.size() >= limitNbCubes && m_inFlight == 0) break;
    if (timeUp && m_inFlight == 0) break;
  }

  // drain remaining queue into cubes
  while (!queue.empty()) {
    cubes.push_back(queue.top().lits);
    queue.pop();
  }

  double elapsed = std::chrono::duration<double>(
                       std::chrono::steady_clock::now() - tStart)
                       .count();
  std::cerr << "c Done. " << cubes.size() << " cubes in " << elapsed << "s\n";

  return !unsat;
}

// ── getPartialOrderComputed ── (copy-pasted verbatim) ────────────────────────

void CubeGeneratorCLI::getPartialOrderComputed(
    std::vector<unsigned>& partialOrder) const {
  partialOrder.clear();
  partialOrder.resize(m_nbVariable + 1, 1);
  if (m_partialOrderHeuristic) {
    for (int i = 1; i <= (int)m_nbVariable; i++)
      partialOrder[i] = (unsigned)m_partialOrderHeuristic->getLevel(i);
  }
}

}  // namespace discount

