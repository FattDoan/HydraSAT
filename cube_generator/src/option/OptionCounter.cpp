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

#include "OptionCounter.hpp"

#include <iostream>  // FIXME: delte after debug
namespace discount {

const char* OptionCounter::s_counterMethod[] = {"d4", "SharpSatTD"};
const char* OptionCounter::s_partialOrderCounter[] = {"share", "compute", "no"};

/**
 * @brief OptionCounter::OptionCounter implementation.
 */
OptionCounter::OptionCounter(const std::string& path,
                             const std::string& counterMethod,
                             const OptionCubeGenerator& optionCubeGenerator,
                             const PartialOrderCounter& partialOrderCounter) {
  m_pathFormula = path;
  if (counterMethod == "d4" || counterMethod == "D4")
    m_counterMethod = COUNTER_D4;
  else if (counterMethod == "SharpSatTD" || counterMethod == "sstd")
    m_counterMethod = COUNTER_SHARPSATTD;
  else
    throw std::runtime_error(counterMethod + "is not an available counter.");

  m_optionCubeGenerator = optionCubeGenerator;
  m_optionMethodType = option::METHOD_COUNTER;
  m_partialOrderCounter = partialOrderCounter;
}  // constructor

/**
 * @brief OptionCounter::~OptionCounter implementation.
 */
OptionCounter::~OptionCounter() {}  // destructor

/**
 * @brief OptionCounter::display implementation.
 */
void OptionCounter::display(std::ostream& out) {
  out << "c The instance we are considering is: " << m_pathFormula << '\n';
  out << "c The underlying counter used is: ";
  out << s_counterMethod[m_counterMethod] << "\n";
  out << "c The partial order heuristic used by the counter: ";
  out << s_partialOrderCounter[m_partialOrderCounter] << "\n";
  out << "c The preprocessing used is: ";
  out << s_preprocMethod[m_preprocMethod] << '\n';
  out << "c The budget given for computing the backbone is: "
      << m_preprocTimeout << '\n';

  m_optionCubeGenerator.display(out);
}  // display

/**
 * @brief OptionCounter::helpPartialOrderCounterHeuristic implementation.
 */
void OptionCounter::helpPartialOrderCounterHeuristic(std::ostream& out,
                                                     const std::string& head) {
  for (unsigned i = 0;
       i < sizeof(s_partialOrderCounter) / sizeof(s_partialOrderCounter[0]);
       i++) {
    out << head << s_partialOrderCounter[i] << " ";
    if (i == DEFAULT_PARTIAL_ORDER_COUNTER) out << "(default)";
    out << '\n';
  }
}  // helpPartialOrderCounterHeuristic

/**
 * @brief OptionCounter::helpCounterMethod implementation.
 */
void OptionCounter::helpCounterMethod(std::ostream& out,
                                      const std::string& head) {
  for (unsigned i = 0; i < sizeof(s_counterMethod) / sizeof(s_counterMethod[0]);
       i++) {
    out << head << s_counterMethod[i] << " ";
    if (i == DEFAULT_COUNTER) out << "(default)";
    out << '\n';
  }
}  // helpPartialOrderCounterHeuristic

}  // namespace discount