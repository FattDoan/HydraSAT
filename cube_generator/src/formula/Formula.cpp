
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

#include "Formula.hpp"

#include "WeightedCNF.hpp"

namespace discount {

/**
 * @brief Formula::getWeightsInFloat implementation.
 */
void Formula::getWeightsInFloat(std::vector<mpz::mpf_float> &weightLit,
                                std::vector<mpz::mpf_float> &weightVar) {
  if (getType() == MP_FLOAT) {
    weightLit =
        static_cast<WeightedCNF<mpz::mpf_float> *>(this)->getWeightLit();
    weightVar =
        static_cast<WeightedCNF<mpz::mpf_float> *>(this)->getWeightVar();
  } else {
    for (auto &w :
         static_cast<WeightedCNF<mpz::mpz_int> *>(this)->getWeightLit()) {
      weightLit.push_back(mpz::mpf_float(w));
    }
    for (auto &w :
         static_cast<WeightedCNF<mpz::mpz_int> *>(this)->getWeightVar()) {
      weightVar.push_back(mpz::mpf_float(w));
    }
  }
}  // getWeightsInFloat

}  // namespace discount