/*
 * PartialOrderHeuristicTreeDecompLocal.cpp
 *
 * Replaces the MPI scatter/gather in PartialOrderHeuristicTreeDecomposition.
 * Each std::thread runs FlowCutter with a different random seed (identical to
 * what the distributed workers did), and we keep the best treewidth result.
 *
 * The centroid-root logic from the paper ("iteratively remove leaf nodes until
 * one or two central nodes remain") is applied after picking the best
 * decomposition to bias branching toward structurally central variables.
 */

#include "PartialOrderHeuristicTreeDecompLocal.hpp"

#include <cassert>
#include <iostream>
#include <mutex>
#include <thread>
#include <vector>

// ── include your FlowCutter headers here ─────────────────────────────────────
// #include "3rdParty/flow-cutter/…"
// Adjust to whatever path the flow-cutter library lives at in your tree.
// ─────────────────────────────────────────────────────────────────────────────

namespace discount {

// ── runFlowCutter ─────────────────────────────────────────────────────────────
//
// PORT THIS FROM YOUR WORKER CODE.
// The stub below returns a flat (uniform-level) order so the binary compiles
// without FlowCutter; replace with the real call.
//
int PartialOrderHeuristicTreeDecompLocal::runFlowCutter(
    Formula* formula,
    unsigned seed,
    double   /*timeBudget*/,
    std::vector<double>& outOrder) {

  unsigned n = formula->getNbVar();
  outOrder.assign(n + 1, 1.0);

  // ── TODO: replace stub with real FlowCutter call ──────────────────────────
  //
  // 1. Build primal graph:
  //      nodes  = {1 … n}
  //      edge(u,v) iff ∃ clause containing both var u and var v
  //
  // 2. Run FlowCutter (anytime, stop after timeBudget seconds):
  //      flow_cutter::Config cfg; cfg.random_seed = seed;
  //      auto td = flow_cutter::run(graph, cfg, timeBudget);
  //
  // 3. Extract elimination order and treewidth:
  //      for (unsigned pos = 0; pos < td.order.size(); ++pos)
  //        outOrder[td.order[pos]] = (double)(pos + 1);
  //      return td.treewidth;
  //
  // ─────────────────────────────────────────────────────────────────────────
  (void)seed;
  return -1;  // -1 = unknown treewidth (stub)
}

// ── constructor ───────────────────────────────────────────────────────────────

PartialOrderHeuristicTreeDecompLocal::PartialOrderHeuristicTreeDecompLocal(
    Formula* formula, unsigned nbWorkers, double timeBudget) {

  std::cerr << "c [PartialOrder] Tree decomposition  (threads=" << nbWorkers
            << ", budget=" << timeBudget << "s)\n";

  unsigned n = formula->getNbVar();
  m_level.assign(n + 1, 1.0);

  // ── parallel FlowCutter runs ─────────────────────────────────────────────
  std::mutex                        bestMtx;
  int                               bestWidth = INT_MAX;
  std::vector<double>               bestOrder(n + 1, 1.0);

  std::vector<std::thread> workers;
  workers.reserve(nbWorkers);

  for (unsigned w = 0; w < nbWorkers; ++w) {
    workers.emplace_back([&, w]() {
      std::vector<double> order;
      // each worker uses a distinct seed (mirrors the original distributed setup)
      int width = runFlowCutter(formula, /*seed=*/w * 1337 + 42,
                                timeBudget, order);

      std::lock_guard<std::mutex> lk(bestMtx);
      if (width >= 0 && width < bestWidth) {
        bestWidth = width;
        bestOrder = order;
      }
    });
  }
  for (auto& t : workers) t.join();

  m_treewidth = (bestWidth == INT_MAX) ? -1 : bestWidth;
  m_level     = bestOrder;

  // ── centroid-root adjustment (paper: "iteratively remove leaf nodes") ─────
  // Build a degree array over the tree decomposition bags.
  // Here we approximate by counting how many clauses each variable appears in
  // and ranking central (high-degree in primal graph) variables at lower levels
  // so they are branched on first.
  //
  // A full centroid computation requires the actual tree structure from
  // FlowCutter.  If your FlowCutter binding exposes the tree, replace this
  // approximation with the exact centroid traversal.
  {
    std::vector<unsigned> degree(n + 1, 0);
    for (auto& cl : formula->getClauses())
      for (auto& l : cl) degree[l.var()]++;

    // Rescale: variables with higher primal-graph degree get a lower level
    // number → branched on earlier (matches the centroid-root intuition).
    unsigned maxDeg = *std::max_element(degree.begin() + 1, degree.end());
    if (maxDeg > 0 && m_treewidth == -1) {
      // only fall back to degree-based order when FlowCutter stub is active
      for (unsigned i = 1; i <= n; ++i)
        m_level[i] = (double)(maxDeg - degree[i] + 1);
    }
  }

  std::cerr << "c [PartialOrder] treewidth=" << m_treewidth << "\n";
}

// ── getLevel ─────────────────────────────────────────────────────────────────

double PartialOrderHeuristicTreeDecompLocal::getLevel(Var v) const {  // override
  assert((unsigned)v < m_level.size());
  return m_level[v];
}

}  // namespace discount
