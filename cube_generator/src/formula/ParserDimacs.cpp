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

#include "ParserDimacs.hpp"

#include "WeightedCNF.hpp"
#include "src/utils/BufferRead.hpp"
#include "src/utils/Parsing.hpp"

namespace discount {
namespace mpz = boost::multiprecision;

struct DataFormula {
  std::vector<mpz::mpf_float> weightLit;
  std::vector<std::vector<Lit>> clauses;
  std::vector<Var> projectedVar;
  int nbVars;
};

/**
 * @brief Parses input files to extract features of the WCNF (Weighted CNF)
 * formula.
 *
 * This function reads from the input stream to retrieve the weights for each
 * literal, the set of clauses, and the set of projected variables associated
 * with the WCNF formula.
 *
 * @param[in] in is the stream where the formula comes from.
 * @param[out] data is the structure to gather the information about the
 * formula.
 */
void parse_DIMACS_main(BufferRead &in, DataFormula &data) {
  std::vector<Lit> lits;
  std::string s;

  int nbClauses = 0;

  for (;;) {
    in.skipSpace();
    if (in.eof()) break;

    if (in.currentChar() == 'p') {
      in.consumeChar();
      in.skipSpace();

      bool vpActivated = false;
      if (in.currentChar() == 'p') {
        vpActivated = true;
        in.consumeChar();
      }
      if (in.currentChar() == 'w') in.consumeChar();

      if (in.nextChar() != 'c' || in.nextChar() != 'n' || in.nextChar() != 'f')
        std::cerr << "PARSE ERROR! Unexpected char: " << in.currentChar()
                  << "\n",
            exit(3);

      data.nbVars = in.nextInt();
      nbClauses = in.nextInt();

      if (vpActivated)
        std::cout << "c Some variable are marked: " << in.nextInt() << "\n";
      data.weightLit.resize(((data.nbVars + 1) << 1), 1);

      if (nbClauses < 0) printf("parse error\n"), exit(2);
    } else if (in.currentChar() == 'c') {
      in.consumeChar();
      in.skipSimpleSpace();

      if (in.currentChar() == 'p') {
        in.consumeChar();
        if (in.canConsume("weight")) {
          Parsing::parseNextWeightedLits(in, data.weightLit);

          // in this format we have an end line we have to consume.
          [[maybe_unused]] int endLine = in.nextInt();
          assert(!endLine);
        } else if (in.canConsume("show"))
          Parsing::readListIntTerminatedByZero(in, data.projectedVar);
        else
          in.skipLine();
      } else {
        in.skipLine();
      }
    } else {
      lits.clear();
      int v = -1;
      do {
        v = in.nextInt();
        if ((v > 0 && data.nbVars < v) || (-v > 0 && data.nbVars < -v))
          std::cerr << "PARSE ERROR! Number of variables incorrect: " << v
                    << "\n",
              exit(3);

        if (v)
          lits.push_back((v > 0) ? Lit::makeLit(v, false)
                                 : Lit::makeLit(-v, true));
      } while (v);

      assert(lits.size());
      std::sort(lits.begin(), lits.end());

      // remove redundant literal and check for tautology.
      unsigned j = 1;
      bool isSat = false;
      for (unsigned i = 1; !isSat && i < lits.size(); i++) {
        if (lits[i] == lits[j - 1]) continue;
        isSat = lits[i] == ~lits[j - 1];
        lits[j++] = lits[i];
      }

      // add the clause only if not SAT.
      if (!isSat) {
        lits.resize(j);
        data.clauses.push_back(lits);
      }
    }
  }
}  // parse_DIMACS_main

/**
 * @brief ParserDimacs::parserDIMAC implementation.
 */
Formula *ParserDimacs::parserDIMACS(const std::string &input_stream) {
  BufferRead in(input_stream);

  DataFormula data;
  parse_DIMACS_main(in, data);

  // check if is float or int weighted CNF
  bool isInt = true;
  for (auto w : data.weightLit)
    if (mpz::round(w) != w) {
      isInt = false;
      break;
    }

  // return the correct formula.
  if (isInt) {
    std::vector<mpz::mpz_int> weights;
    for (auto w : data.weightLit)
      weights.push_back(mpz::mpz_int(mpz::round(w)));

    return new WeightedCNF<mpz::mpz_int>(data.nbVars, weights,
                                         data.projectedVar, data.clauses);
  }

  return new WeightedCNF<mpz::mpf_float>(data.nbVars, data.weightLit,
                                         data.projectedVar, data.clauses);
}  // parse_DIMACS

}  // namespace discount