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

#include "src/formula/Formula.hpp"
#include "src/formula/ProblemTypes.hpp"
#include "src/option/OptionCubeGenerator.hpp"

namespace discount {
/**
 * @class PartialOrderHeuristic
 * @brief This abstract class represents a partial order heuristic for variable
 * ordering.
 *
 * @details Each derived class must provide an implementation for the
 * `getMinStrata` method, which returns the minimum strata based on the given
 * list of candidates.
 *
 * @note This class is abstract, meaning that it cannot be instantiated
 * directly. Instead, instances of derived classes should be created using
 * the `makePartialOrderHeuristic` factory method, which takes an options object
 * as an argument.
 */
class PartialOrderHeuristic {
 public:
  /**
   * @brief To be sure the derived destructor will be called.
   */
  virtual ~PartialOrderHeuristic() {}

  /**
   * @brief Static factory method for creating an instance of this heuristic
   * with the given options.
   *
   * @param[in] formula The formula object for which to generate partial order
   * heuristics.
   * @param[in] idRoot The ID of the root node in the search tree.
   * @param[in] idWorkers The IDs of the worker threads.
   * @param[in] options The options for the heuristic, which may include the
   * cube generator and any additional configuration.
   *
   * @return A new instance of the derived class, constructed using the given
   * options.
   */
  static PartialOrderHeuristic *makePartialOrderHeuristic(
      Formula *formula, int idRoot, std::vector<int> idWorkers,
      const OptionCubeGenerator &options);

  /**
   * @brief Virtual method for getting the level of a given variable.
   *
   * @param[in] v The variable for which to get the level.
   *
   * @return The level of the variable `v`.
   */
  virtual double getLevel(Var v) = 0;
};

}  // namespace discount