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

#include "OptionCubeGenerator.hpp"

namespace discount {

/**
 * @brief Declaration of the static elements.
 */
const char *OptionCubeGenerator::s_branchingHeuristiCubeGenerator[] = {
    "random", "vsads", "vsids", "dlcs", "mom", "jwts"};
const char *OptionCubeGenerator::s_partialOrderHeuristicCubeGenerator[] = {
    "none", "tree-decomposition"};

/**
 * @brief OptionCubeGenerator::OptionCubeGenerator implementation.
 */
OptionCubeGenerator::OptionCubeGenerator(
    const std::string &path, unsigned nbCube,
    const BranchingCubeHeuristic branchingHeuristic,
    const PartialOrderCubeHeuristic partialOrderHeuristic, unsigned budget) {
  m_pathFormula = path;
  m_nbCubeByWorker = nbCube;
  m_optionMethodType = option::METHOD_CUBE_GENERATOR;
  m_branchingHeuristic = branchingHeuristic;
  m_partialOrderHeuristic = partialOrderHeuristic;
  m_budgetTreeDecomp = budget;
}  // constructor

/**
 * @brief OptionCubeGenerator::~OptionCubeGenerator implementation.
 */
OptionCubeGenerator::~OptionCubeGenerator() {}  // destructor

/**
 * @brief OptionCubeGenerator::display implementation.
 */
void OptionCubeGenerator::display(std::ostream &out) {
  out << "c The number of cubes by worker is: " << m_nbCubeByWorker << '\n';
  out << "c The approach used for generating the cubes is:\n"
      << "c\t-branching heuristic: "
      << s_branchingHeuristiCubeGenerator[m_branchingHeuristic] << '\n'
      << "c\t-partial order heuristic: "
      << s_partialOrderHeuristicCubeGenerator[m_partialOrderHeuristic] << '\n'
      << "c\t-budget (in seconds) for computing the partial order: "
      << m_budgetTreeDecomp << '\n';
}  // display

/**
 * @brief OptionCubeGenerator::helpBranchingCubeHeuristic implementation.
 */
void OptionCubeGenerator::helpBranchingCubeHeuristic(std::ostream &out,
                                                     const std::string &head) {
  for (unsigned i = 0; i < sizeof(s_branchingHeuristiCubeGenerator) /
                               sizeof(s_branchingHeuristiCubeGenerator[0]);
       i++) {
    out << head << s_branchingHeuristiCubeGenerator[i] << " ";
    if (i == DEFAULT_CUBE_GENERATOR_BRANCHING_HEURISTIC) out << "(default)";
    out << '\n';
  }
}  // helpBranchingCubeHeuristic

/**
 * @brief OptionCubeGenerator::helpPartialOrderCubeHeuristicstd implementation.
 */
void OptionCubeGenerator::helpPartialOrderCubeHeuristic(
    std::ostream &out, const std::string &head) {
  for (unsigned i = 0; i < sizeof(s_partialOrderHeuristicCubeGenerator) /
                               sizeof(s_partialOrderHeuristicCubeGenerator[0]);
       i++) {
    out << head << s_partialOrderHeuristicCubeGenerator[i] << " ";
    if (i == DEFAULT_CUBE_GENERATOR_PARTIAL_ORDER_HEURISTIC) out << "(default)";
    out << '\n';
  }
}  // helpPartialOrderCubeHeuristicstd

}  // namespace discount