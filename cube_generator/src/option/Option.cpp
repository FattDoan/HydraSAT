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

#include "Option.hpp"

#include <iostream>
#include <map>
#include <string>

#include "OptionCounter.hpp"
#include "OptionCubeGenerator.hpp"

namespace discount {

const char *Option::s_preprocMethod[] = {"none", "equiv"};

/**
 * @brief Option::help implementation.
 */
void Option::help(int argc, char **argv) {
  std::cout << "[USAGE] mpirun -np NB_PROC " << argv[0]
            << " -m METHOD -i CNF [OPTIONS]\n\n";
  std::cout << "\tNB_PROC: The number of processes to launch. Must be greater "
               "than 2.\n";
  std::cout << "\tMETHOD: The method to execute (either 'cube-generator' or "
               "'counter').\n";
  std::cout << "\tCNF: The file path to a formula in CNF format.\n\n";

  std::cout << "Available methods:\n";
  std::cout << "\t-cube-generator: Generates a set of cubes only.\n";
  std::cout << "\t-counter: Counts the number of models using a "
               "cube-and-conquer approach.\n\n";

  std::cout << "Available OPTIONS:\n";
  std::cout << "\t-verbosity BOOL: set the verbosity (default: false)\n";

  std::cout << "\t-preproc STRING: Set the preprocessing method used.\n";
  for (unsigned i = 0; i < NB_PREPROC; i++)
    std::cout << "\t\t* " << s_preprocMethod[i]
              << ((i == DEFAULT_PREPROC) ? " (default)\n" : "\n");
  std::cout << "\t-preproc-timeout INT: Set the timeout for the preprocessing "
               "method used (default"
            << DEFAULT_PREPROC_TIMEOUT << ").\n";

  std::cout << "\t-cube-byWorker INT: Specifies the number of cubes per worker "
            << "(default: " << OptionCubeGenerator::DEFAULT_NB_CUBES_BY_WORKER
            << ").\n";

  std::cout << "\t-cube-generator-branching-heuristic STRING: Specifies the "
               "heuristic used for generating the cubes:\n";
  OptionCubeGenerator::helpBranchingCubeHeuristic(std::cout, "\t\t* ");

  std::cout << "\t-cube-generator-partial-order-heuristic STRING: "
               "Specifies the heuristic used for generating the cubes:\n";
  OptionCubeGenerator::helpPartialOrderCubeHeuristic(std::cout, "\t\t* ");

  std::cout << "\t-cube-generator-partial-order-budget INT: Specifies the "
               "budget allocated for computing the partial order "
            << "(default: " << OptionCubeGenerator::DEFAULT_BUDGET_TREE_DECOMP
            << ")\n";
  std::cout
      << "\t-counter-name STRING: Specifies the underlying counter to use:\n";
  // std::cout << "\t\t* D4\t(default)\n";
  OptionCounter::helpCounterMethod(std::cout, "\t\t* ");

  std::cout << "\t-counter-partial-order-heuristic STRING: "
               "Specifies the partial order heuristic for the counter:\n";
  OptionCounter::helpPartialOrderCounterHeuristic(std::cout, "\t\t* ");
}  // help

template <typename T, size_t SIZE>
int getEnumValue(const T (&array)[SIZE], const std::string &s, int argc,
                 char **argv) {
  for (int i = 0; i < (int)SIZE; i++) {
    if (array[i] == s) return i;
  }

  Option::help(argc, argv);
  throw std::runtime_error("Error: the given option" + s + " is incorrect!");
  return 0;
}  // getEnumValue

/**
 * @brief Option::parse implementation.
 */
Option *Option::parse(int argc, char **argv) {
  std::map<std::string, std::string> mapOption;

  // we ensure we have pair of parameter (optio, value).
  if ((argc - 1) & 1) {
    help(argc, argv);
    return NULL;
  }

  // parse the command line.
  for (int i = 1; i < argc; i += 2)
    mapOption[argv[i]] = std::string(argv[i + 1]);

  if ((mapOption.find("-i") == mapOption.end()) ||
      (mapOption.find("-m") == mapOption.end())) {
    help(argc, argv);
    return NULL;
  }

  // default options.
  unsigned budget = OptionCubeGenerator::DEFAULT_BUDGET_TREE_DECOMP;
  unsigned nbCubeByWorker = OptionCubeGenerator::DEFAULT_NB_CUBES_BY_WORKER;
  OptionCubeGenerator::BranchingCubeHeuristic branchingHeuristicCubeGen =
      OptionCubeGenerator::DEFAULT_CUBE_GENERATOR_BRANCHING_HEURISTIC;
  OptionCubeGenerator::PartialOrderCubeHeuristic partialOrderHeuristicCubeGen =
      OptionCubeGenerator::DEFAULT_CUBE_GENERATOR_PARTIAL_ORDER_HEURISTIC;
  OptionCounter::PartialOrderCounter partialOrderCounterHeuristic =
      OptionCounter::DEFAULT_PARTIAL_ORDER_COUNTER;
  bool verbosity = false;
  Option::PreprocMethod preproc = Option::DEFAULT_PREPROC;
  unsigned preprocTimeout = Option::DEFAULT_PREPROC_TIMEOUT;

  if (mapOption.find("-cube-byWorker") != mapOption.end())
    nbCubeByWorker = std::stoi(mapOption["-cube-byWorker"]);

  // cubes:
  if (mapOption.find("-cube-generator-branching-heuristic") != mapOption.end())
    branchingHeuristicCubeGen =
        (OptionCubeGenerator::BranchingCubeHeuristic)getEnumValue(
            OptionCubeGenerator::s_branchingHeuristiCubeGenerator,
            mapOption["-cube-generator-branching-heuristic"], argc, argv);

  if (mapOption.find("-verbosity") != mapOption.end()) {
    verbosity = mapOption["-verbosity"] == "true";
    std::cout << "c set the verbosity to " << verbosity << '\n';
  }

  if (mapOption.find("-cube-generator-partial-order-heuristic") !=
      mapOption.end())
    partialOrderHeuristicCubeGen =
        (OptionCubeGenerator::PartialOrderCubeHeuristic)getEnumValue(
            OptionCubeGenerator::s_partialOrderHeuristicCubeGenerator,
            mapOption["-cube-generator-partial-order-heuristic"], argc, argv);

  // counters:
  if (mapOption.find("-counter-partial-order-heuristic") != mapOption.end())
    partialOrderCounterHeuristic =
        (OptionCounter::PartialOrderCounter)getEnumValue(
            OptionCounter::s_partialOrderCounter,
            mapOption["-counter-partial-order-heuristic"], argc, argv);

  if (mapOption.find("-cube-generator-partial-order-budget") !=
      mapOption.end()) {
    budget = std::stoi(mapOption["-cube-generator-partial-order-budget"]);
  }

  if (mapOption.find("-preproc") != mapOption.end()) {
    preproc = (Option::PreprocMethod)getEnumValue(
        Option::s_preprocMethod, mapOption["-preproc"], argc, argv);
  }
  if (mapOption.find("-preproc-timeout") != mapOption.end()) {
    preprocTimeout = std::stoi(mapOption["-preproc-timeout"]);
  }

  // the method we want to run.
  if (mapOption["-m"] == "cube-generator") {
    Option *ret = new OptionCubeGenerator(mapOption["-i"], nbCubeByWorker,
                                          branchingHeuristicCubeGen,
                                          partialOrderHeuristicCubeGen, budget);
    ret->verbosity = verbosity;
    ret->m_preprocMethod = preproc;
    ret->m_preprocTimeout = preprocTimeout;
    return ret;
  } else if (mapOption["-m"] == "counter") {
    std::string counterName = mapOption.find("-counter-name") == mapOption.end()
                                  ? "D4"
                                  : mapOption["-counter-name"];

    OptionCubeGenerator given(nbCubeByWorker, branchingHeuristicCubeGen,
                              partialOrderHeuristicCubeGen, budget);
    given.verbosity = verbosity;
    given.m_preprocMethod = preproc;
    Option *ret = new OptionCounter(mapOption["-i"], counterName, given,
                                    partialOrderCounterHeuristic);
    ret->verbosity = verbosity;
    ret->m_preprocMethod = preproc;
    ret->m_preprocTimeout = preprocTimeout;
    return ret;
  }

  help(argc, argv);
  return NULL;
}  // parse
}  // namespace discount