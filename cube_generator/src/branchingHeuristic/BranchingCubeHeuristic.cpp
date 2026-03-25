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

#include "BranchingCubeHeuristic.hpp"

#include "D4BranchingCubeHeuristic.hpp"
#include "RandomBranchingCubeHeuristic.hpp"

namespace discount {

/**
 * @brief BranchingCubeHeuristic::makeBranchingCubeHeuristic implementation.
 */
BranchingCubeHeuristic *BranchingCubeHeuristic::makeBranchingCubeHeuristic(
    Formula *formula, const OptionCubeGenerator &options) {
  switch (options.getBranchingHeuristic()) {
    case OptionCubeGenerator::BranchingCubeHeuristic::RANDOM:
      return new RandomBranchingCubeHeuristic(formula);
    case OptionCubeGenerator::BranchingCubeHeuristic::VSADS:
    case OptionCubeGenerator::BranchingCubeHeuristic::VSIDS:
    case OptionCubeGenerator::BranchingCubeHeuristic::MOM:
    case OptionCubeGenerator::BranchingCubeHeuristic::DLCS:
    case OptionCubeGenerator::BranchingCubeHeuristic::JWTS:
      return new D4BranchingCubeHeuristic(formula,
                                          options.getBranchingHeuristic());
    default:
      throw std::runtime_error("Error: the given option is incorrect!");
  }

  return NULL;
}  // makeBranchingCubeHeuristic

}  // namespace discount