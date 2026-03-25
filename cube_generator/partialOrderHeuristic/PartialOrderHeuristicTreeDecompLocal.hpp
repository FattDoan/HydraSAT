/*
 * PartialOrderHeuristicTreeDecompLocal.hpp
 * Replaces the MPI-distributed PartialOrderHeuristicTreeDecomposition.
 * Spawns `nbWorkers` std::threads each running FlowCutter with a distinct
 * random seed; picks the result with the lowest treewidth (paper: §cube gen).
 */
#pragma once

#include <vector>
#include "src/formula/Formula.hpp"
#include "src/formula/ProblemTypes.hpp"

namespace discount {

class PartialOrderHeuristicTreeDecompLocal {
 public:
  /**
   * @param formula    The CNF whose primal graph is decomposed.
   * @param nbWorkers  How many parallel FlowCutter threads to run.
   *                   More threads → better treewidth within same time budget.
   * @param timeBudget Seconds each FlowCutter thread may run (anytime algo).
   */
  PartialOrderHeuristicTreeDecompLocal(Formula* formula,
                                       unsigned nbWorkers  = 4,
                                       double   timeBudget = 10.0);

  double getLevel(Var v) const;
  int    getTreewidth() const { return m_treewidth; }

 private:
  std::vector<double> m_level;   // index 0 unused; index i = level of var i
  int                 m_treewidth = -1;

  /**
   * Build primal graph of formula, run FlowCutter with `seed`,
   * populate `outOrder` (variable → level) and return treewidth.
   *
   * ── HOW TO FILL THIS IN ──────────────────────────────────────────────────
   * In your original codebase the worker process that receives
   * MSG_TASK_TREE_DECOMP does exactly this job.  Port that code here.
   * Typical FlowCutter library call:
   *
   *   flow_cutter::Config cfg;
   *   cfg.random_seed = seed;
   *   auto result = flow_cutter::compute_decomposition(graph, cfg, timeBudget);
   *   // result.order → permutation of vars
   *   // result.treewidth
   *
   * Then convert the elimination order to levels:
   *   for (unsigned i = 0; i < order.size(); ++i)
   *     outOrder[order[i]] = (double)(i + 1);
   * ─────────────────────────────────────────────────────────────────────────
   */
  static int runFlowCutter(Formula* formula,
                           unsigned seed,
                           double   timeBudget,
                           std::vector<double>& outOrder);
};

}  // namespace discount
