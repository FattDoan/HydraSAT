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

#include <boost/math/special_functions/math_fwd.hpp>
#include <boost/multiprecision/gmp.hpp>

#include "Formula.hpp"
#include "ProblemTypes.hpp"
#include "src/utils/BufferRead.hpp"

namespace discount {

class ParserDimacs {
 public:
  /**
   * @brief Parses the file specified by the given filename and returns the
   * formula it contains.
   *
   * This function reads the input file and extracts the formula described
   * within.
   *
   * @param fileName The name of the input file containing the formula.
   *
   * @return The formula described in the input file.
   */
  static Formula *parserDIMACS(const std::string &fileName);
};
}  // namespace discount
