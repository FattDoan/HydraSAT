cmake_minimum_required(VERSION 3.16)
project(cube_cli CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# ── adjust these paths to match your discount source tree ────────────────────
set(DISCOUNT_ROOT "${CMAKE_CURRENT_SOURCE_DIR}")

include_directories(
  ${DISCOUNT_ROOT}                          # discount:: headers
  ${DISCOUNT_ROOT}/src                      # discount:: src
  
  ${DISCOUNT_ROOT}/3rdParty/d4/src
  ${DISCOUNT_ROOT}/3rdParty/d4
  ${DISCOUNT_ROOT}/3rdParty/d4/3rdParty/bipe/src
  ${DISCOUNT_ROOT}/3rdParty/d4/3rdParty/bipe

  # add your FlowCutter include dir here:
  ${DISCOUNT_ROOT}/3rdParty/d4/3rdParty/flowCutter/src
)

# ── sources (branching heuristic files are unchanged from the original) ───────
set(SOURCES
  main.cpp
  CubeGeneratorCLI.cpp
  partialOrderHeuristic/PartialOrderHeuristicTreeDecompLocal.cpp

  # unchanged from original:
  ${DISCOUNT_ROOT}/src/branchingHeuristic/D4BranchingCubeHeuristic.cpp
  ${DISCOUNT_ROOT}/src/branchingHeuristic/BranchingCubeHeuristic.cpp
)

add_executable(cube_cli ${SOURCES})

# link against whatever discount / d4 / bipe / flow-cutter libraries you have
target_link_libraries(cube_cli
  PRIVATE
    # discount_lib    # uncomment if you build discount as a lib
    # d4_lib
    # bipe_lib
    Threads::Threads
)
find_package(Threads REQUIRED)
