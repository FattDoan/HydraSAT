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

#include <ostream>

namespace discount {

namespace option {
enum OptionMethodType { METHOD_CUBE_GENERATOR, METHOD_COUNTER };

}

class Option {
 public:
  enum PreprocMethod { PREPROC_NONE, PREPROC_EQUIV, NB_PREPROC };
  static const char *s_preprocMethod[2];
  static const PreprocMethod DEFAULT_PREPROC = PREPROC_EQUIV;
  static const unsigned DEFAULT_PREPROC_TIMEOUT = 5;

 protected:
  std::string m_pathFormula;
  option::OptionMethodType m_optionMethodType;
  PreprocMethod m_preprocMethod = DEFAULT_PREPROC;
  unsigned m_preprocTimeout = DEFAULT_PREPROC_TIMEOUT;

 public:
  bool verbosity = false;

  /**
   * @brief Virtual destructor to ensure proper cleanup in derived classes.
   */
  virtual ~Option() = default;

  /**
   * @brief Displays the status of the current options.
   *
   * @param out The output stream where the options' status will be printed.
   */
  virtual void display(std::ostream &out) = 0;

  /**
   * @brief Parses command-line arguments to create an options object.
   *
   * This function processes the provided command-line parameters and generates
   * an `Option` object based on the specified arguments.
   *
   * @param argc The number of command-line arguments.
   * @param argv The list of command-line arguments.
   * @return A pointer to an `Option` object representing the parsed options.
   */
  static Option *parse(int argc, char **argv);

  /**
   * @brief Retrieves the file path of the formula.
   *
   * @return A constant reference to the formula file path.
   */
  inline const std::string &getPathCnf() { return m_pathFormula; }

  /**
   * @brief Retrieves the type of method being used.
   *
   * @return The method type encoded by this option.
   */
  inline const option::OptionMethodType getMethodType() const {
    return m_optionMethodType;
  }  // getMethodType

  /**
   * @brief Retrieves the preprocessing type of method being used.
   *
   * @return The preprocessing method gien by this option.
   */
  inline const PreprocMethod getPreproc() const { return m_preprocMethod; }

  /**
   * @brief Retrieves the budget given for computing the backbone.
   *
   * @return The budget given.
   */
  inline const unsigned getPreprocTimeout() const { return m_preprocTimeout; }

  /**
   * @brief Displays the help information for the command-line interface.
   *
   * This function outputs a detailed help message, providing guidance on how to
   * use the program and its available command-line options.
   *
   * @param[in] argc The number of arguments provided on the command line.
   * @param[in] argv An array of arguments passed via the command line.
   */
  static void help(int argc, char **argv);
};
}  // namespace discount