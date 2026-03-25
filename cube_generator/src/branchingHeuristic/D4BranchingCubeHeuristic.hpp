#pragma once

#include <vector>
// FIX #2: inherit PartialOrderHeuristic so the pointer converts cleanly
#include "partialOrderHeuristic/PartialOrderHeuristic.hpp"
#include "src/formula/Formula.hpp"
#include "src/formula/ProblemTypes.hpp"

namespace discount {

class PartialOrderHeuristicTreeDecompLocal : public PartialOrderHeuristic {
 public:
  PartialOrderHeuristicTreeDecompLocal(Formula* formula,
                                       unsigned nbWorkers  = 4,
                                       double   timeBudget = 10.0);

  // implements the pure virtual from PartialOrderHeuristic
  double getLevel(Var v) const override;
  int    getTreewidth() const { return m_treewidth; }

 private:
  std::vector<double> m_level;
  int                 m_treewidth = -1;

  static int runFlowCutter(Formula* formula,
                           unsigned seed,
                           double   timeBudget,
                           std::vector<double>& outOrder);
};

}  // namespace discount
