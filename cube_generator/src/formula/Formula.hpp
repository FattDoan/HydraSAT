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

#include <boost/multiprecision/gmp.hpp>

#include "ProblemTypes.hpp"

namespace discount {

namespace mpz = boost::multiprecision;
enum FormulaTypeEnum { MP_NONE, MP_INT, MP_FLOAT };

class Formula {
 protected:
  std::vector<std::vector<Lit>> m_clauses;
  std::vector<Var> m_projectedVars;
  bool m_isUnsat = false;
  unsigned m_nbVar;

 public:
  /**
   * @brief Default constructor. We make it virtual in order to make possible
   * the definition of the destructor in the derived classes.
   */
  virtual ~Formula() {}

  /**
   * @brief Return the type of the formula. This type should one listed in the
   * enum type FormulaTypeEnum.
   *
   * @return FormulaTypeEnum
   */
  virtual FormulaTypeEnum getType() = 0;

  /**
   * @brief Diplay the formula together with the information about the weighted
   * literals and variables in a given output stream.
   *
   * @param out is the stream where are printed out the information.
   */
  virtual void display(std::ostream &out) = 0;

  /**
   * @brief Encodes a list of weighted literals into a string format for
   * decoding.
   *
   * This function serializes a list of weighted literals into a string,
   * enabling reconstruction of the original list from its encoded form. To
   * associate each substring with its correct weight, a list of pairs `(l, s)`
   * is generated, where `l` represents the literal, and `s` specifies the
   * number of digits encoding the weight. This metadata is organized in a row
   * format to facilitate transmission to other workers.
   *
   * @param[out] weights The serialized string representing the list of literals
   * and weights.
   * @param[out] coupleLitSizeStr The list of pairs (literal, size) associating
   * literals with their weight lengths.
   */
  virtual void serializeWeightedLits(std::vector<char> &weights,
                                     std::vector<int> &coupleLitSizeStr) = 0;

  /**
   * @brief Get the matrix of the formula.
   *
   * @return a list of clauses.
   */
  inline std::vector<std::vector<Lit>> &getClauses() { return m_clauses; }

  /**
   * @brief Set the CNF matrix.
   *
   * @param clauses is the new set of clauses.
   */
  inline void setClauses(std::vector<std::vector<Lit>> &clauses) {
    m_clauses = clauses;
  }  // setClauses

  /**
   * @brief Accessor to the number of variables.
   *
   * @return the number of variables of the CNF formula.
   */
  inline unsigned getNbVar() { return m_nbVar; }

  /**
   * @brief Set the number of variables of the CNF formula.
   *
   * @param n is the new number of variables.
   */
  inline void setNbVar(int n) { m_nbVar = n; }

  /**
   * @brief Get the list of projected variables.
   *
   * @return a vector of variables.
   */
  inline std::vector<Var> &getProjectedVar() { return m_projectedVars; }

  /**
   * @brief Retrieves the weights of literals and variables as vectors of
   * `mpf_float`.
   *
   * This function populates the provided vectors with the weights of literals
   * and variables in floating-point format.
   *
   * @param[out] weightLit A vector to store the weights of literals.
   * @param[out] weightVar A vector to store the weights of variables.
   */
  void getWeightsInFloat(std::vector<mpz::mpf_float> &weightLit,
                         std::vector<mpz::mpf_float> &weightVar);

  inline void getInfoFormula() {
    std::cout << "c Information about the formula:\n";
    std::cout << "c Number of variables: " << m_nbVar << "\n";
    std::cout << "c Number of clauses: " << m_clauses.size() << "\n";
  }
};
}  // namespace discount