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

#include "PartialOrderHeuristic.hpp"

#include "PartialOrderHeuristicNone.hpp"
#include "PartialOrderHeuristicTreeDecomposition.hpp"

namespace discount {
/**
 * @brief PartialOrderHeuristic::makePartialOrderHeuristic implementation.
 */
PartialOrderHeuristic *PartialOrderHeuristic::makePartialOrderHeuristic(
    Formula *formula, int idRoot, std::vector<int> idWorkers,
    const OptionCubeGenerator &options) {
  switch (options.getPartialOrderHeuristic()) {
    case OptionCubeGenerator::PartialOrderCubeHeuristic::NONE:
      return new PartialOrderHeuristicNone();
    case OptionCubeGenerator::PartialOrderCubeHeuristic::TREE_DECOMPOSITION:
      return new PartialOrderHeuristicTreeDecomposition(formula, idRoot,
                                                        idWorkers);
    default:
      throw std::runtime_error("Error: the given option is incorrect!");
  }

  return NULL;
}  // makePartialOrderHeuristic
}  // namespace discount