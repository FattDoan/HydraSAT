/*
 * discount
 * Copyright (C) 2024  Univ. Artois & CNRS
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with this library; if not, write to the Free Software Foundation,
 * Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301 USA
 */

#include "D4BranchingCubeHeuristic.hpp"

#include "3rdParty/d4/src/configurations/ConfigurationBranchingHeuristic.hpp"
#include "src/formula/WeightedCNF.hpp"

namespace discount {
/**
 * @brief D4BranchingCubeHeuristic::D4BranchingCubeHeuristic implementation.
 */
D4BranchingCubeHeuristic::D4BranchingCubeHeuristic(
    Formula *formula,
    const OptionCubeGenerator::BranchingCubeHeuristic option) {
  std::cout << "c [D4 Branching Heuristic] Constructor\n";
  m_nbVariables = formula->getNbVar();
  m_marked.resize(formula->getNbVar() + 1, false);

  assert(formula->getType() != MP_NONE);

  std::vector<mpz::mpf_float> weightLit, weightVar;
  formula->getWeightsInFloat(weightLit, weightVar);
  m_problemManager =
      new d4::ProblemManagerCnf(formula->getNbVar(), weightLit, weightVar);

  std::vector<std::vector<d4::Lit>> clauses;
  for (auto &cl : formula->getClauses()) {
    clauses.push_back(std::vector<d4::Lit>());
    for (auto &l : cl)
      clauses.back().push_back(d4::Lit::makeLit(l.var(), l.sign()));
  }

  m_problemManager->setClauses(clauses);

  m_cnfManager = new d4::CnfManagerDyn(*m_problemManager);

  m_solver = new d4::WrapperMinisat();
  m_solver->initSolver(*m_problemManager);

  std::vector<d4::Var> vars;
  for (unsigned i = 1; i <= m_nbVariables; i++) vars.push_back(i);
  m_solver->warmStart(29, 11, vars, std::cout);

  d4::ConfigurationBranchingHeuristic d4ConfigBranchingHeuristic;
  switch (option) {
    case OptionCubeGenerator::BranchingCubeHeuristic::VSADS:
      d4ConfigBranchingHeuristic.scoringMethodType =
          d4::ScoringMethodType::SCORE_VSADS;
      break;
    case OptionCubeGenerator::BranchingCubeHeuristic::VSIDS:
      d4ConfigBranchingHeuristic.scoringMethodType =
          d4::ScoringMethodType::SCORE_VSIDS;
      break;
    case OptionCubeGenerator::BranchingCubeHeuristic::MOM:
      d4ConfigBranchingHeuristic.scoringMethodType =
          d4::ScoringMethodType::SCORE_MOM;
      break;
    case OptionCubeGenerator::BranchingCubeHeuristic::DLCS:
      d4ConfigBranchingHeuristic.scoringMethodType =
          d4::ScoringMethodType::SCORE_DLCS;
      break;
    case OptionCubeGenerator::BranchingCubeHeuristic::JWTS:
      d4ConfigBranchingHeuristic.scoringMethodType =
          d4::ScoringMethodType::SCORE_JWTS;
      break;
    default:
      throw std::runtime_error("Error: the given option is supported!");
  }

  d4::OptionBranchingHeuristic d4OptionBranchingHeuristic(
      d4ConfigBranchingHeuristic);
  m_heuristic = new d4::BranchingHeuristicClassic(
      d4OptionBranchingHeuristic, m_problemManager, m_cnfManager, *m_solver,
      *m_solver, std::cout);
}  // constructor

/**
 * @brief D4BranchingCubeHeuristic::~D4BranchingCubeHeuristic implementation.
 */
D4BranchingCubeHeuristic::~D4BranchingCubeHeuristic() {
  delete m_heuristic;
  delete m_problemManager;
  delete m_cnfManager;
  delete m_solver;
}  // destructor

/**
 * @brief D4BranchingCubeHeuristic::next implementation.
 */
Var D4BranchingCubeHeuristic::next(const std::vector<Lit> &lits,
                                   PartialOrderHeuristic *partialOrder) {
  // modify the cnf formula
  std::vector<d4::Lit> tod4;
  for (auto &l : lits) tod4.push_back(d4::Lit::makeLit(l.var(), l.sign()));
  m_cnfManager->preUpdate(tod4);

  // mark the literals of the cube.
  for (auto &l : lits) m_marked[l.var()] = true;

  // collect the set of variables.
  std::vector<d4::Var> variables;
  for (unsigned i = 1; i <= m_nbVariables; i++) {
    if (m_marked[i]) continue;
    variables.push_back(i);
  }

  // unmark!
  for (auto &l : lits) m_marked[l.var()] = false;

  if (!variables.size()) {
    m_cnfManager->postUpdate(tod4);
    return var_Undef;
  }
  d4::ListLit d4Lits;
  m_heuristic->selectLitSet(variables, d4Lits);
  assert(d4Lits.size() == 1);

  m_cnfManager->postUpdate(tod4);
  return d4Lits[0].var();
}  // next
}  // namespace discount