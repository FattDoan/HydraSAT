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

#include "../../partialOrderHeuristic/PartialOrderHeuristic.hpp"
#include "src/formula/ProblemTypes.hpp"
#include "src/option/OptionCubeGenerator.hpp"

namespace discount {
/**
 * @class BranchingCubeHeuristic
 * @brief This abstract class represents a heuristic that generates branching
 * cubes to split the search space.
 *
 * @details Each derived class must provide an implementation for the
 * `makeBranchingCube` method, which generates a single branching cube.
 *
 * @note This class is abstract, meaning that it cannot be instantiated
 * directly. Instead, instances of derived classes should be created using
 * the `makeBranchingCubeHeuristic` static factory method.
 */
class BranchingCubeHeuristic {
 public:
  virtual ~BranchingCubeHeuristic() {}

  /**
   * @brief Static factory method for creating an instance of this heuristic
   * with the given options.
   *
   * @param[in] formula The formula object for which to generate branching
   * heuristic to generate the cubes.
   * @param[in] options The options for the cube generator used by this
   * heuristic.
   * @return A new instance of the derived class, constructed using the given
   * options.
   */
  static BranchingCubeHeuristic *makeBranchingCubeHeuristic(
      Formula *formula, const OptionCubeGenerator &options);

  /**
   * @brief Virtual method for returning the next variable to be assigned a
   * value.
   *
   * @param[in] lits The list of literals of the cube under consideration.
   * @param[in] partialOder Give the level of the variables.
   *
   * @return The next variable to be assigned a value. If candidates is empty,
   * then Var_Undef is returned.
   */
  virtual Var next(const std::vector<Lit> &lits,
                   PartialOrderHeuristic *partialOrder) = 0;
};
}  // namespace discount
