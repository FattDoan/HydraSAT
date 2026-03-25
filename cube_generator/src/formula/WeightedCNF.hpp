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

namespace discount {
namespace mpz = boost::multiprecision;

template <class T>
class WeightedCNF : public Formula {
 protected:
  std::vector<T> m_weightLit;
  std::vector<T> m_weightVar;

 public:
  /**
   * @brief Constructs a formula with a clause matrix, initializing the
   * required information about variables, literal weights, and projected
   * variables.
   *
   * This constructor initializes a formula structure with an empty clause
   * matrix, specifying the number of variables, weights for each literal,
   * projected variables, and the set of clauses.
   *
   * @param nbVar The number of variables in the formula.
   * @param weightLit The weights associated with each literal.
   * @param projected The list of projected variables.
   * @param clauses The set of clauses in the formula.
   */
  WeightedCNF(int nbVar, const std::vector<T> &weightLit,
              const std::vector<Var> &projected,
              const std::vector<std::vector<Lit>> &clauses) {
    m_nbVar = nbVar;
    m_weightLit = weightLit;
    m_projectedVars = projected;
    m_clauses = clauses;

    m_weightVar.resize(nbVar + 1);
    for (int i = 0; i <= nbVar; i++)
      m_weightVar[i] = weightLit[i << 1] + weightLit[(i << 1) | 1];
  }  // constructor

  /**
   * @brief Destroy the object by cleaning all the vectors.
   */
  ~WeightedCNF() {
    m_clauses.clear();
    m_weightLit.clear();
    m_weightVar.clear();
    m_projectedVars.clear();
    m_nbVar = 0;
  }  // destructor.

  /**
   * @brief Diplay the formula together with the information about the weighted
   * literals and variables in a given output stream.
   *
   * @param out is the stream where are printed out the information.
   */
  void display(std::ostream &out) {
    out << "weight list: ";
    for (unsigned i = 1; i <= m_nbVar; i++) {
      Lit l = Lit::makeLit(i, false);
      out << i << "[" << m_weightVar[i] << "] ";
      out << l << "(" << m_weightLit[l.intern()] << ") ";
      out << ~l << "(" << m_weightLit[(~l).intern()] << ") ";
    }
    out << "\n";

    out << "projected var: ";
    for (auto v : getProjectedVar()) out << v << " ";
    out << "\n";

    out << "p cnf " << m_nbVar << " " << m_clauses.size() << "\n";
    for (auto cl : m_clauses) {
      for (auto &l : cl) out << l << " ";
      out << "0\n";
    }
  }  // display

  /**
   * @brief Display some statistics about the formula in a given output stream.
   *
   * @param out is the stream where are printed out the information.
   * @param startLine is a prefix for each printed lines.
   */
  void displayStat(std::ostream &out, std::string startLine) {
    unsigned nbLits = 0;
    unsigned nbBin = 0;
    unsigned nbTer = 0;
    unsigned nbUnit = 0;
    unsigned nbMoreThree = 0;

    for (auto &c : m_clauses) {
      nbLits += c.size();
      if (c.size() == 1) nbUnit++;
      if (c.size() == 2) nbBin++;
      if (c.size() == 3) nbTer++;
      if (c.size() > 3) nbMoreThree++;
    }

    out << startLine << "Number of variables: " << m_nbVar << "\n";
    out << startLine << "Number of clauses: " << m_clauses.size() << "\n";
    out << startLine << "Number of unit clauses: " << nbUnit << "\n";
    out << startLine << "Number of binary clauses: " << nbBin << "\n";
    out << startLine << "Number of ternary clauses: " << nbTer << "\n";
    out << startLine << "Number of clauses larger than 3: " << nbMoreThree
        << "\n";
    out << startLine << "Number of literals: " << nbLits << "\n";
  }  // displayStat

  /**
   * @brief Create an unsatisfiable problem.
   *
   * @return a formula which is unsatisfiable.
   */
  WeightedCNF *getUnsatProblem() {
    WeightedCNF *ret = new WeightedCNF(m_nbVar, m_weightLit, m_projectedVars,
                                       std::vector<std::vector<Lit>>());
    ret->m_isUnsat = true;

    std::vector<Lit> cl;
    Lit l = Lit::makeLit(1, false);

    cl.push_back(l);
    ret->getClauses().push_back(cl);

    cl[0] = l.neg();
    ret->getClauses().push_back(cl);

    return ret;
  }  // getUnsatProblem

  /**
   * @brief Given a list of unit literals, this function simplify the matrix.
   *
   * @param units is the list of unit literals.
   *
   * @return a new formula that consists in the simplication of the formula.
   */
  WeightedCNF *getConditionedFormula(std::vector<Lit> &units) {
    WeightedCNF *ret = new WeightedCNF(m_nbVar, m_weightLit, m_projectedVars,
                                       std::vector<std::vector<Lit>>());

    std::vector<char> value(m_nbVar + 1, 0);
    for (auto l : units) {
      value[l.var()] = l.sign() + 1;
      ret->getClauses().push_back({l});
    }

    for (auto cl : m_clauses) {
      // get the simplified clause.
      std::vector<Lit> scl;
      bool isSAT = false;
      for (auto l : cl) {
        if (!value[l.var()]) scl.push_back(l);

        isSAT = l.sign() + 1 == value[l.var()];
        if (isSAT) break;
      }

      // add the simplified clause if needed.
      if (!isSAT) ret->getClauses().push_back(scl);
    }

    return ret;
  }  // getConditionedFormula

  /**
   * @brief Get a 'map' the for each literal give its weight.
   *
   * @return the list of weight associated with each literal (here we consider
   * the inter representation of the literals).
   */
  inline std::vector<T> &getWeightLit() { return m_weightLit; }

  /**
   * @brief Get a 'map' that aossociate for each variable its weight.
   *
   * @return the list of weight.
   */
  inline std::vector<T> &getWeightVar() { return m_weightVar; }

  /**
   * @brief Get the weight associated with a given literal.
   *
   * @param l is the literal we look for its weight.
   * @return the weight associated to l.
   */
  inline const T &getWeightLit(Lit l) {
    return m_weightLit[l.intern()];
  }  // getWeightLit

  /**
   * @brief Get the weight associated with a given variables.
   *
   * @param v is the variable we are looking for its weight.
   * @return the weight associated with v.
   */
  inline const T &getWeightVar(Var v) { return m_weightVar[v]; }

  /**
   * @brief Ask if the formula is UNSAT.
   *
   * @return true if the formula is UNSAT, false otherwise.
   */
  inline bool isUnsat() { return m_isUnsat; }

  /**
   * @brief Set the status of the formula.
   *
   * @param b is a boolean set to true if the formula is SAT, and false
   * otherwise.
   */
  inline void isUnsat(bool b) { m_isUnsat = b; }

  /**
   * @brief Ask if the formula use weights they are floats.
   *
   * @return true if it uses float, false otherwise.
   */
  inline bool isFloat() {
    return std::is_same_v<T, mpz::mpf_float>;
  }  // isFloat

  /**
   * @brief Compute the value for free and unit variables.
   *
   * @param[in] units are the units literals.
   * @param[in] frees are the free variables.
   *
   * \return the right value.
   */
  inline T computeWeightUnitFree(std::vector<Lit> &units,
                                 std::vector<Var> &frees) {
    T tmp = 1;
    for (auto &l : units) {
      assert(l.intern() < m_weightLit.size());
      tmp *= T(m_weightLit[l.intern()]);
    }
    for (auto &v : frees) {
      assert(v < (int)m_weightVar.size());
      tmp *= T(m_weightVar[v]);
    }

    return tmp;
  }  // computeWeightUnitFree

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
  void serializeWeightedLits(std::vector<char> &weights,
                             std::vector<int> &coupleLitSizeStr) {
    for (unsigned i = 1; i <= getNbVar(); i++) {
      for (Lit l : {Lit::makeLitTrue(i), Lit::makeLitFalse(i)}) {
        if (getWeightLit(l) == 1)
          continue;
        else {
          const std::string &tmp = getWeightLit(l).str();
          coupleLitSizeStr.push_back(l.human());
          coupleLitSizeStr.push_back(tmp.size());

          for (auto &c : tmp) weights.push_back(c);
        }
      }
    }
  }  // serializeWeightedLits

  /**
   * @brief Returns the formula type based on the result of the isFloat
   * function.
   *
   * If `isFloat` returns `false`, this function returns `MP_INT`; if `isFloat`
   * returns `true`, it returns `MP_FLOAT`.
   *
   * @return The type of the formula, either `MP_INT` or `MP_FLOAT`.
   */
  FormulaTypeEnum getType() {
    if (isFloat())
      return MP_FLOAT;
    else
      return MP_INT;
  }  // getType
};

}  // namespace discount