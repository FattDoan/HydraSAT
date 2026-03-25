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
#pragma once

#include "Option.hpp"

namespace discount {
class OptionCubeGenerator : public Option {
 public:
  /**
   * @brief Enum representing the different methods for generating cubes.
   *
   * This enumeration defines the available strategies for partitioning
   * the search space when solving a formula.
   *
   * - RANDOM: Generates cubes randomly.
   * - TREE_DECOMPOSITION: Uses a tree decomposition approach to create cubes,
   *   aiming for a more structured and efficient partitioning.
   */
  enum BranchingCubeHeuristic {
    RANDOM,
    VSADS,
    VSIDS,
    DLCS,
    MOM,
    JWTS,
    NB_BRANCHING_HEURISTIC
  };
  static const char *s_branchingHeuristiCubeGenerator[6];

  enum PartialOrderCubeHeuristic {
    NONE,
    TREE_DECOMPOSITION,
    NB_PARTIAL_ORDER_HEURISTIC
  };
  static const char *s_partialOrderHeuristicCubeGenerator[2];

  static unsigned const DEFAULT_NB_CUBES_BY_WORKER = 30;
  static BranchingCubeHeuristic const
      DEFAULT_CUBE_GENERATOR_BRANCHING_HEURISTIC = VSADS;
  static PartialOrderCubeHeuristic const
      DEFAULT_CUBE_GENERATOR_PARTIAL_ORDER_HEURISTIC = TREE_DECOMPOSITION;
  static unsigned const DEFAULT_BUDGET_TREE_DECOMP = 10;

 private:
  unsigned m_budgetTreeDecomp = DEFAULT_BUDGET_TREE_DECOMP;
  unsigned m_nbCubeByWorker = DEFAULT_NB_CUBES_BY_WORKER;
  BranchingCubeHeuristic m_branchingHeuristic =
      DEFAULT_CUBE_GENERATOR_BRANCHING_HEURISTIC;
  PartialOrderCubeHeuristic m_partialOrderHeuristic =
      DEFAULT_CUBE_GENERATOR_PARTIAL_ORDER_HEURISTIC;

 public:
  /**
   * @brief The defualt constructor.
   *
   */
  OptionCubeGenerator() = default;

  /**
   * @brief Constructs an instance with a specified path and cube allocation per
   * worker.
   *
   * @param path The file path to the instance.
   * @param nbCube The number of cubes assigned to each worker.
   * @param branchingHeuristic The branching heuristic used for generating the
   * cubes.
   * @param partialOrderHeuristic The method that deal with the partial order.
   * @param budget The budget given (in seconds) to compute the partial order.
   */
  OptionCubeGenerator(const std::string &path, unsigned nbCube,
                      const BranchingCubeHeuristic branchingHeuristic,
                      const PartialOrderCubeHeuristic partialOrderHeuristic,
                      unsigned budget);

  /**
   * @brief Constructs an instance with a specified path and cube allocation per
   * worker.
   *
   * @param nbCube The number of cubes assigned to each worker.
   * @param branchingHeuristic The branching heuristic used for generating the
   * cubes.
   * @param partialOrderHeuristic The method that deal with the partial order.
   * @param budget The budget given (in seconds) to compute the partial order.
   */
  OptionCubeGenerator(unsigned nbCube,
                      const BranchingCubeHeuristic branchingHeuristic,
                      const PartialOrderCubeHeuristic partialOrderHeuristic,
                      unsigned budget)
      : OptionCubeGenerator("/dev/null", nbCube, branchingHeuristic,
                            partialOrderHeuristic, budget) {}

  /**
   * @brief Destructor to ensure proper cleanup.
   */
  ~OptionCubeGenerator();

  /**
   * @brief Displays the status of the current options.
   *
   * @param out The output stream where the options' status will be printed.
   */
  void display(std::ostream &out) override;

  /**
   * @brief Retrieves the budget (in seconds) for running the tool for computing
   * the tree decomposition.
   *
   * @return The budget.
   */
  inline unsigned getBudgetTreeDecomp() const { return m_budgetTreeDecomp; }

  /**
   * @brief Retrieves the number of cubes assigned per worker.
   *
   * @return The number of cubes each worker processes.
   */
  inline unsigned getNbCubeByWorker() { return m_nbCubeByWorker; }

  /**
   * @brief Retrieves the selected branching heuristic method for cube
   * generation.
   *
   * @return An enum value of OptionCubeGenerator::BranchingHeuristic specifying
   * the heuristic.
   */
  inline BranchingCubeHeuristic getBranchingHeuristic() const {
    return m_branchingHeuristic;
  }

  /**
   * @brief Retrieves the selected partial order branching heuristic method for
   * cube generation.
   *
   * @return An enum value of OptionCubeGenerator::PartialOrderHeuristic
   * specifying the heuristic.
   */
  inline PartialOrderCubeHeuristic getPartialOrderHeuristic() const {
    return m_partialOrderHeuristic;
  }

  /**
   * @brief Generates output for the cube generation branching heuristic.
   *
   * @param out The output stream to print to.
   * @param head The prefix string to prepend to each output line.
   */
  static void helpBranchingCubeHeuristic(std::ostream &out,
                                         const std::string &head);

  /**
   * @brief Generates output for the cube generation branching heuristic.
   *
   * @param out The output stream to print to.
   * @param head The prefix string to prepend to each output line.
   */
  static void helpPartialOrderCubeHeuristic(std::ostream &out,
                                            const std::string &head);
};
}  // namespace discount