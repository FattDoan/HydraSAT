/*
 * cube-cli  –  standalone cube generator (no MPI)
 * Reads a DIMACS CNF on stdin or --input, writes JSON to stdout.
 *
 * JSON schema
 * -----------
 * {
 *   "satisfiable": bool,          // false  →  UNSAT detected during splitting
 *   "cubes": [                    // each cube is an array of DIMACS literals
 *     [1, -3, 7, ...],
 *     ...
 *   ],
 *   "partial_order": [0,1,2,...], // level per variable (index = var, 1-based, index 0 unused)
 *   "stats": {
 *     "nb_cubes":     int,
 *     "time_seconds": float,
 *     "treewidth":    int         // -1 if tree-decomp was skipped
 *   }
 * }
 */

#include <cassert>
#include <chrono>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>
#include <boost/math/special_functions/math_fwd.hpp>
#include <boost/multiprecision/gmp.hpp>
#include "CubeGeneratorCLI.hpp"
#include "src/formula/Formula.hpp"           // discount::Formula (abstract)
#include "src/formula/WeightedCNF.hpp"       // discount::WeightedCNF<T> (concrete)
#include "src/formula/ProblemTypes.hpp"       // discount::Lit

using namespace discount;
 
// ─── tiny DIMACS parser ──────────────────────────────────────────────────────
//
// Formula is abstract.  For plain unweighted CNF we use the concrete
// WeightedCNF<mpz::mpf_float> with uniform weights (1.0 per literal/variable).
// This is exactly what D4BranchingCubeHeuristic::getWeightsInFloat() expects.
 
static Formula* parseDIMACS(std::istream& in) {
  int nbVar = 0, nbClause = 0;
  std::string line;
 
  while (std::getline(in, line)) {
    if (line.empty() || line[0] == 'c') continue;
    if (line[0] == 'p') {
      std::istringstream ss(line);
      std::string p, cnf;
      ss >> p >> cnf >> nbVar >> nbClause;
      break;
    }
  }
  if (!nbVar) throw std::runtime_error("No 'p cnf' header found");
 
  // Actual constructor: WeightedCNF(nbVar, weightLit, projected, clauses)
  // weightLit layout: index = lit.intern() = var<<1 | sign
  //   positive lit of var v  →  index 2*v
  //   negative lit of var v  →  index 2*v + 1
  // weightVar is computed internally as weightLit[2v] + weightLit[2v+1].
  // For plain unweighted CNF every weight is 1.0.
  std::vector<mpz::mpf_float> weightLit(2 * (nbVar + 1), mpz::mpf_float(1));
 
  // projected: empty = all variables are free (standard CNF)
  std::vector<Var> projected;
 
  // parse clauses first, then pass them to the constructor all at once
  std::vector<std::vector<Lit>> clauses;
  std::vector<Lit> clause;
  int lit;
  while (in >> lit) {
    if (lit == 0) {
      clauses.push_back(std::move(clause));
      clause.clear();
    } else {
      int  v   = std::abs(lit);
      bool neg = lit < 0;
      clause.push_back(Lit::makeLit(v, neg));
    }
  }
  // flush a trailing clause without a terminating 0 (defensive)
  if (!clause.empty()) clauses.push_back(std::move(clause));
 
  return new WeightedCNF<mpz::mpf_float>(nbVar, weightLit, projected, clauses);
}
 
// ─── JSON helpers (no external dependency) ───────────────────────────────────
 
static void emitJSON(const std::vector<std::vector<Lit>>& cubes,
                     const std::vector<unsigned>& partialOrder,
                     bool satisfiable,
                     double elapsed,
                     int treewidth) {
  std::cout << "{\n";
  std::cout << "  \"satisfiable\": " << (satisfiable ? "true" : "false") << ",\n";
 
  std::cout << "  \"cubes\": [\n";
  for (size_t ci = 0; ci < cubes.size(); ++ci) {
    std::cout << "    [";
    for (size_t li = 0; li < cubes[ci].size(); ++li) {
      const Lit& l = cubes[ci][li];
      int dimacsLit = l.sign() ? -(int)l.var() : (int)l.var();
      std::cout << dimacsLit;
      if (li + 1 < cubes[ci].size()) std::cout << ", ";
    }
    std::cout << "]";
    if (ci + 1 < cubes.size()) std::cout << ",";
    std::cout << "\n";
  }
  std::cout << "  ],\n";
 
  std::cout << "  \"partial_order\": [";
  for (size_t i = 0; i < partialOrder.size(); ++i) {
    std::cout << partialOrder[i];
    if (i + 1 < partialOrder.size()) std::cout << ", ";
  }
  std::cout << "],\n";
 
  std::cout << "  \"stats\": {\n";
  std::cout << "    \"nb_cubes\": "     << cubes.size()          << ",\n";
  std::cout << "    \"time_seconds\": " << elapsed               << ",\n";
  std::cout << "    \"treewidth\": "    << treewidth             << "\n";
  std::cout << "  }\n";
  std::cout << "}\n";
}
 
// ─── main ─────────────────────────────────────────────────────────────────────
 
static void usage(const char* prog) {
  std::cerr << "Usage: " << prog
            << " [--input FILE] [--cubes N] [--threads T] [--time S]\n"
            << "  --input   path to DIMACS CNF  (default: stdin)\n"
            << "  --cubes   target cube count    (default: 1000)\n"
            << "  --threads parallel SAT workers (default: 4)\n"
            << "  --time    budget in seconds    (default: 60)\n";
}
 
int main(int argc, char* argv[]) {
  std::string inputFile;
  unsigned limitCubes = 1000;
  unsigned nbThreads  = 4;
  double   timeBudget = 60.0;
 
  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if ((arg == "--input" || arg == "-i") && i + 1 < argc)
      inputFile = argv[++i];
    else if ((arg == "--cubes" || arg == "-n") && i + 1 < argc)
      limitCubes = std::stoul(argv[++i]);
    else if ((arg == "--threads" || arg == "-t") && i + 1 < argc)
      nbThreads = std::stoul(argv[++i]);
    else if ((arg == "--time" || arg == "-s") && i + 1 < argc)
      timeBudget = std::stod(argv[++i]);
    else if (arg == "--help" || arg == "-h") { usage(argv[0]); return 0; }
    else { usage(argv[0]); return 1; }
  }
 
  // ── parse formula ──
  Formula* formula = nullptr;
  try {
    if (inputFile.empty()) {
      formula = parseDIMACS(std::cin);
    } else {
      std::ifstream f(inputFile);
      if (!f) throw std::runtime_error("Cannot open: " + inputFile);
      formula = parseDIMACS(f);
    }
  } catch (const std::exception& e) {
    std::cerr << "Parse error: " << e.what() << "\n";
    return 2;
  }
 
  // ── generate ──
  auto t0 = std::chrono::steady_clock::now();
 
  CubeGeneratorCLI gen(nbThreads);
  std::vector<std::vector<Lit>> cubes;
  bool satisfiable = gen.generate(formula, cubes, limitCubes, timeBudget);
 
  double elapsed = std::chrono::duration<double>(
                       std::chrono::steady_clock::now() - t0)
                       .count();
 
  std::vector<unsigned> partialOrder;
  gen.getPartialOrderComputed(partialOrder);
 
  // ── emit JSON ──
  emitJSON(cubes, partialOrder, satisfiable, elapsed, gen.getTreewidth());
 
  delete formula;
  return 0;
}
