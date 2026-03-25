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

#include <vector>

#include "Option.hpp"
#include "OptionCubeGenerator.hpp"

namespace discount {

class OptionCounter : public Option {
 public:
  enum CounterMethod { COUNTER_D4, COUNTER_SHARPSATTD, NB_COUNTER };
  static const char* s_counterMethod[2];
  static const CounterMethod DEFAULT_COUNTER = COUNTER_D4;

  enum PartialOrderCounter {
    PARTIAL_ORDER_SHARE,
    PARTIAL_ORDER_COMPUTE,
    PARTIAL_ORDER_NO,
    NB_PARTIAL_ORDER
  };

  static const PartialOrderCounter DEFAULT_PARTIAL_ORDER_COUNTER =
      PARTIAL_ORDER_COMPUTE;

  static const char* s_partialOrderCounter[3];

 private:
  CounterMethod m_counterMethod;
  OptionCubeGenerator m_optionCubeGenerator;
  PartialOrderCounter m_partialOrderCounter = DEFAULT_PARTIAL_ORDER_COUNTER;
  std::vector<double> m_partialOrder;
  double m_scaleFactor;

 public:
  /**
   * @brief Constructs an instance with the specified formula path, counter, and
   * cube generator options.
   *
   * @param path The file path to the formula to be solved.
   * @param counterName The name of the counter to be used.
   * @param optionCubeGenerator Configuration options for the cube generator.
   * @param partialOrderCounter The partial order heuristic by the counters.
   */
  OptionCounter(const std::string& path, const std::string& counterMethod,
                const OptionCubeGenerator& optionCubeGenerator,
                const PartialOrderCounter& partialOrderCounter);

  /**
   * @brief Destructor to ensure proper cleanup in derived classes.
   */
  ~OptionCounter();

  /**
   * @brief Displays the status of the current options.
   *
   * @param out The output stream where the options' status will be printed.
   */
  void display(std::ostream& out);

  /**
   * @brief Retrieves the type of counter method to be used.
   *
   * @return The current counter method as an `OptionCounterMethod` object.
   */
  inline CounterMethod getCounterMethod() const {
    return m_counterMethod;
  }  // getCounterMethod

  /**
   * @brief Sets the counter method parameter.
   *
   * @param[in] counterMethod The counter method to set.
   */
  inline void setCounterMethod(const CounterMethod& counterMethod) {
    m_counterMethod = counterMethod;
  }  // setCounterMethod

  /**
   * @brief Generates output for the counter method.
   *
   * @param out The output stream to print to.
   * @param head The prefix string to prepend to each output line.
   */
  static void helpCounterMethod(std::ostream& out, const std::string& head);

  /**
   * @brief Retrieves the options related to cube generation.
   *
   * @return An `OptionCubeGenerator` object containing the cube generation
   * settings.
   */
  inline OptionCubeGenerator getOptionCubeGenerator() const {
    return m_optionCubeGenerator;
  }  // getOptionCubeGenerator

  /**
   * @brief Generates output for the counter partial order heuristic.
   *
   * @param out The output stream to print to.
   * @param head The prefix string to prepend to each output line.
   */
  static void helpPartialOrderCounterHeuristic(std::ostream& out,
                                               const std::string& head);

  /**
   * @brief Retrieves the options related to partial order used by the counter.
   *
   * @return An `PartialOrderCounter` object containing the  counter settings.
   */
  inline PartialOrderCounter getPartialOrderCounter() const {
    return m_partialOrderCounter;
  }  // getPartialOrderCounter

  /**
   * @brief Retrieves the partial order stored.
   *
   * @return a vector consisting of the level for each variables.
   */
  inline const std::vector<double>& getPartialOrder() const {
    return m_partialOrder;
  }  // getPartialOrder

  /**
   * @brief Sets the partial order for the heuristic.
   *
   * This function updates the internal partial order with the provided order.
   *
   * @param[in] order A vector representing the partial order to be assigned.
   */
  inline void setPartialOrder(std::vector<double>& order) {
    m_partialOrder = order;
  }  // setPartialOrder

  /**
   * @brief Retrieves the current scale factor.
   *
   * @return The scale factor as a double.
   */
  inline const double getScaleFactor() const { return m_scaleFactor; }

  /**
   * @brief Sets the scale factor to a specified value.
   *
   * @param[in] v The new scale factor value.
   */
  inline void setScaleFactor(double v) { m_scaleFactor = v; }
};
}  // namespace discount