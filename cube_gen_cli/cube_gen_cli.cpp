/*
 * cube_gen_cli.cpp — HydraSAT Cube Generator
 *
 * Adapts the DisCount CubeGenerator (Univ. Artois & CNRS, 2024) as a
 * standalone CLI subprocess.  Class names, method signatures, and the core
 * generate / createCubes / initPropagator / initQueue logic are taken
 * directly from the paper's source.  Three layers that require MPI or
 * external libraries are replaced with self-contained equivalents:
 *
 *   bipe::reducer::Propagator      → Propagator  (same interface, BCP only)
 *   D4BranchingCubeHeuristic       → JWTSBranchingCubeHeuristic
 *   PartialOrderHeuristicTreeDecomp → PartialOrderHeuristicMinDegree
 *   TaskSolverSAT + MPI workers    → direct BCP inside createCubes()
 *
 * Build (no external deps):
 *   g++ -O3 -std=c++17 cube_gen_cli.cpp -o cube_gen
 *
 * Usage:
 *   ./cube_gen <formula.cnf> [--num-cubes N] [--timeout S]
 *              [--alpha F] [--strategy combined|jwts|mindeg]
 *              [--output FILE] [--verbose]
 *
 * Output: JSON on stdout (or --output FILE)
 *   { "variable_order":[...], "variable_scores":[...],
 *     "initial_cubes":[[...]], "stats":{...} }
 */

#include <algorithm>
#include <cassert>
#include <chrono>
#include <climits>
#include <cmath>
#include <fstream>
#include <functional>
#include <iostream>
#include <numeric>
#include <queue>
#include <set>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>

namespace discount {

// ─────────────────────────────────────────────────────────────────────────
//  Lit / Var  — identical to the paper's types
// ─────────────────────────────────────────────────────────────────────────

using Var = int;
const Var var_Undef = -1;

struct Lit {
  int x;
  static Lit makeLit(Var v, bool sign) { return {sign ? -v : v}; }
  static Lit makeLitTrue(Var v)        { return { v}; }
  static Lit makeLitFalse(Var v)       { return {-v}; }
  Var  var()  const { return std::abs(x); }
  bool sign() const { return x < 0; }
};

struct Cube {
  std::vector<Lit> lits;
  double weight;           // paper uses (double)lits.size()
};

// Paper's PriorityQueue type: min-heap on weight.
using PriorityQueue = std::priority_queue<
    Cube, std::vector<Cube>, std::function<bool(Cube, Cube)>>;

// ─────────────────────────────────────────────────────────────────────────
//  Formula  — wraps parsed CNF (replaces discount::Formula)
// ─────────────────────────────────────────────────────────────────────────

struct Formula {
  int nbVar = 0;
  std::vector<std::vector<Lit>> clauses;
  int  getNbVar()   const { return nbVar; }
  const std::vector<std::vector<Lit>>& getClauses() const { return clauses; }
};

Formula parseCNF(const std::string& path) {
  std::ifstream f(path);
  if (!f) throw std::runtime_error("Cannot open: " + path);
  Formula formula;
  std::string line;
  while (std::getline(f, line)) {
    if (line.empty() || line[0] == 'c') continue;
    if (line[0] == 'p') {
      std::istringstream ss(line);
      std::string p, cnf; int v, cl;
      ss >> p >> cnf >> v >> cl;
      formula.nbVar = v;
      continue;
    }
    std::istringstream ss(line);
    std::vector<Lit> cl;
    int x;
    while (ss >> x && x != 0)
      cl.push_back(Lit::makeLit(std::abs(x), x < 0));
    if (!cl.empty()) formula.clauses.push_back(std::move(cl));
  }
  return formula;
}

// ─────────────────────────────────────────────────────────────────────────
//  Propagator  — replaces bipe::reducer::Propagator
//
//  Same interface the paper calls:
//    restart() / value(Lit) / uncheckedEnqueue(Lit) / propagate()
//    getIsUnsat() / getTrail() / getTrailSize() / cancelUntilPos(int)
// ─────────────────────────────────────────────────────────────────────────

class Propagator {
 public:
  Propagator(int nbVar, const std::vector<std::vector<Lit>>& clauses)
      : m_n(nbVar), m_clauses(clauses),
        m_assign(nbVar + 1, 0),
        m_watches(2 * (nbVar + 1)) {
    buildWatches();
  }

  void restart() {
    for (Lit l : m_trail) m_assign[l.var()] = 0;
    m_trail.clear();
    m_qhead = 0;
    m_unsat = false;
  }

  // Returns 2 = unassigned, 1 = true, 0 = false.
  // Paper checks: "if (m_propagator->value(lb) > 1)" → only enqueue if unset.
  int value(Lit l) const {
    int a = m_assign[l.var()];
    if (a == 0) return 2;
    return (l.sign() ? (a == -1) : (a == 1)) ? 1 : 0;
  }

  void uncheckedEnqueue(Lit l) {
    m_assign[l.var()] = l.sign() ? -1 : 1;
    m_trail.push_back(l);
  }

  bool propagate() {
    while (m_qhead < (int)m_trail.size()) {
      Lit p = m_trail[m_qhead++];
      if (!propagateLit(p)) { m_unsat = true; return false; }
    }
    return true;
  }

  bool getIsUnsat()   const { return m_unsat; }
  Lit* getTrail()           { return m_trail.data(); }
  int  getTrailSize() const { return (int)m_trail.size(); }

  void cancelUntilPos(int pos) {
    while ((int)m_trail.size() > pos) {
      m_assign[m_trail.back().var()] = 0;
      m_trail.pop_back();
    }
    if (m_qhead > pos) m_qhead = pos;
    m_unsat = false;
  }

 private:
  int m_n;
  const std::vector<std::vector<Lit>>& m_clauses;
  std::vector<int>  m_assign;
  std::vector<Lit>  m_trail;
  int  m_qhead = 0;
  bool m_unsat = false;
  std::vector<std::vector<int>> m_watches;  // [lit_idx] → clause indices

  static int litIdx(Lit l) {
    return l.sign() ? 2 * l.var() + 1 : 2 * l.var();
  }

  void buildWatches() {
    for (int ci = 0; ci < (int)m_clauses.size(); ci++) {
      const auto& c = m_clauses[ci];
      if (c.size() >= 1) m_watches[litIdx(c[0])].push_back(ci);
      if (c.size() >= 2) m_watches[litIdx(c[1])].push_back(ci);
    }
  }

  bool propagateLit(Lit p) {
    Lit falseLit = Lit::makeLit(p.var(), !p.sign());
    auto& ws = m_watches[litIdx(falseLit)];
    int j = 0;
    for (int i = 0; i < (int)ws.size(); i++) {
      const auto& c = m_clauses[ws[i]];
      bool sat = false; int free = 0; Lit unit{}, newW{};
      for (Lit cl : c) {
        int v = value(cl);
        if (v == 1) { sat = true; break; }
        if (v == 2) { if (!free) newW = cl; unit = cl; free++; }
      }
      if (sat) { ws[j++] = ws[i]; continue; }
      if (!free) {
        while (i < (int)ws.size()) ws[j++] = ws[i++];
        ws.resize(j); return false;
      }
      if (free == 1) {
        ws[j++] = ws[i];
        if (value(unit) == 2) uncheckedEnqueue(unit);
      } else {
        m_watches[litIdx(newW)].push_back(ws[i]);
      }
    }
    ws.resize(j); return true;
  }
};

// ─────────────────────────────────────────────────────────────────────────
//  PartialOrderHeuristic  — same abstract interface as the paper
// ─────────────────────────────────────────────────────────────────────────

class PartialOrderHeuristic {
 public:
  virtual ~PartialOrderHeuristic() = default;
  virtual double getLevel(Var v) = 0;

  // Factory — replaces PartialOrderHeuristic::makePartialOrderHeuristic()
  static PartialOrderHeuristic* make(const Formula* f, bool useMinDeg);
};

// Replaces PartialOrderHeuristicNone — all variables same level.
class PartialOrderHeuristicNone : public PartialOrderHeuristic {
 public:
  double getLevel(Var) override { return 1.0; }
};

// Replaces PartialOrderHeuristicTreeDecomposition.
// Uses greedy min-degree elimination on the primal graph (no FlowCutter/MPI).
// Variables with lower elimination order are more "central" → higher level.
class PartialOrderHeuristicMinDegree : public PartialOrderHeuristic {
  std::vector<double> m_level;
 public:
  explicit PartialOrderHeuristicMinDegree(const Formula* formula) {
    int n = formula->getNbVar();
    std::vector<std::set<int>> adj(n + 1);
    for (const auto& c : formula->getClauses())
      for (int i = 0; i < (int)c.size(); i++)
        for (int j = i + 1; j < (int)c.size(); j++) {
          int u = c[i].var(), v = c[j].var();
          if (u != v) { adj[u].insert(v); adj[v].insert(u); }
        }

    m_level.resize(n + 1, 1.0);
    std::vector<bool> elim(n + 1, false);

    for (int step = 1; step <= n; step++) {
      int best = -1, bestDeg = INT_MAX;
      for (int v = 1; v <= n; v++) {
        if (elim[v]) continue;
        int deg = 0;
        for (int u : adj[v]) if (!elim[u]) deg++;
        if (deg < bestDeg) { bestDeg = deg; best = v; }
      }
      if (best < 0) break;
      // Earlier elimination → higher structural importance.
      m_level[best] = (double)(n - step + 1);
      elim[best] = true;
      // Add fill-in edges (same structure as tree decomposition clique step)
      std::vector<int> nbrs;
      for (int u : adj[best]) if (!elim[u]) nbrs.push_back(u);
      for (int i = 0; i < (int)nbrs.size(); i++)
        for (int j = i + 1; j < (int)nbrs.size(); j++) {
          adj[nbrs[i]].insert(nbrs[j]);
          adj[nbrs[j]].insert(nbrs[i]);
        }
    }
    // Normalize to [1..n]
    double mx = *std::max_element(m_level.begin() + 1, m_level.end());
    if (mx > 0)
      for (int v = 1; v <= n; v++)
        m_level[v] = m_level[v] / mx * n + 1.0;
  }
  double getLevel(Var v) override {
    return (v > 0 && v < (int)m_level.size()) ? m_level[v] : 1.0;
  }
};

PartialOrderHeuristic* PartialOrderHeuristic::make(const Formula* f,
                                                    bool useMinDeg) {
  if (useMinDeg) return new PartialOrderHeuristicMinDegree(f);
  return new PartialOrderHeuristicNone();
}

// ─────────────────────────────────────────────────────────────────────────
//  BranchingCubeHeuristic  — same abstract interface as the paper
// ─────────────────────────────────────────────────────────────────────────

class BranchingCubeHeuristic {
 public:
  virtual ~BranchingCubeHeuristic() = default;
  virtual Var next(const std::vector<Lit>& lits,
                   PartialOrderHeuristic* partialOrder) = 0;

  static BranchingCubeHeuristic* make(const Formula* f);
};

// Replaces D4BranchingCubeHeuristic (VSADS/VSIDS/MOM/DLCS/JWTS options).
// We implement JWTS (Jeroslow–Wang Two-Sided) directly — no D4 library.
// score[v] = Σ_{clause c ∋ v} 2^{-|c|}  weighted by partial order level.
class JWTSBranchingCubeHeuristic : public BranchingCubeHeuristic {
  int m_nbVars;
  std::vector<double> m_jwts;
  std::vector<bool>   m_marked;  // scratch buffer (same pattern as D4 impl)
 public:
  explicit JWTSBranchingCubeHeuristic(const Formula* f)
      : m_nbVars(f->getNbVar()),
        m_jwts(f->getNbVar() + 1, 0.0),
        m_marked(f->getNbVar() + 1, false) {
    for (const auto& c : f->getClauses()) {
      double w = std::pow(2.0, -(double)c.size());
      for (Lit l : c) m_jwts[l.var()] += w;
    }
  }

  // Mirrors D4BranchingCubeHeuristic::next():
  //   mark vars in cube → collect unmarked vars → pick best → unmark
  Var next(const std::vector<Lit>& lits,
           PartialOrderHeuristic* po) override {
    // mark the literals of the cube (same comment as paper)
    for (const Lit& l : lits) m_marked[l.var()] = true;

    Var best = var_Undef;
    double bestScore = -1.0;
    for (int v = 1; v <= m_nbVars; v++) {
      if (m_marked[v]) continue;
      double score = m_jwts[v] * (po ? po->getLevel(v) : 1.0);
      if (score > bestScore) { bestScore = score; best = v; }
    }

    // unmark (same pattern as D4BranchingCubeHeuristic::next)
    for (const Lit& l : lits) m_marked[l.var()] = false;
    return best;
  }
};

BranchingCubeHeuristic* BranchingCubeHeuristic::make(const Formula* f) {
  return new JWTSBranchingCubeHeuristic(f);
}

// ─────────────────────────────────────────────────────────────────────────
//  OptionCubeGenerator  — controls generate() parameters
// ─────────────────────────────────────────────────────────────────────────

struct OptionCubeGenerator {
  unsigned numCubes   = 64;
  double   timeoutSec = 30.0;
  double   alpha      = 0.5;  // structural weight (for score export)
  bool     useMinDeg  = true;
  bool     verbose    = false;
};

// ─────────────────────────────────────────────────────────────────────────
//  CubeGenerator  — adapts discount::CubeGenerator
//
//  generate() and createCubes() are structurally identical to the paper.
//  collectTerminatedTask() and assignTasksToWorkers() are removed (no MPI).
//  initPropagator() and initQueue() logic unchanged.
// ─────────────────────────────────────────────────────────────────────────

class CubeGenerator {
 public:
  int pruned = 0;  // observable

  void generate(Formula* formula,
                std::vector<std::vector<Lit>>& cubes,
                const OptionCubeGenerator& options) {
    m_nbVariable = formula->getNbVar();
    m_verbose    = options.verbose;

    // Replaces BranchingCubeHeuristic::makeBranchingCubeHeuristic()
    m_branchingCubeHeuristic = BranchingCubeHeuristic::make(formula);

    // Replaces PartialOrderHeuristic::makePartialOrderHeuristic()
    m_partialOrderHeuristic = PartialOrderHeuristic::make(formula,
                                                          options.useMinDeg);

    // prepare the propagator (same as paper's initPropagator)
    initPropagator(formula);

    // create the cubes — min-heap on weight (same as paper)
    auto cmp = [](Cube c1, Cube c2) { return c1.weight > c2.weight; };
    PriorityQueue queue(cmp);
    initQueue(queue);

    double limitTime = 10.0;
    auto wallStart = std::chrono::steady_clock::now();

    // ADAPTED outer loop — mirrors paper's
    //   "while (queue.size() < limitNbCubes || taskIdxRunning.size())"
    // but without the MPI task-running dimension.
    while ((unsigned)queue.size() < options.numCubes) {
      if (queue.empty()) break;  // nothing to do: UNSAT (same comment, paper)

      double elapsed = std::chrono::duration<double>(
          std::chrono::steady_clock::now() - wallStart).count();
      if (elapsed > options.timeoutSec) {
        if (m_verbose) std::cerr << "c Budget exhausted at " << elapsed << "s\n";
        break;
      }
      if (elapsed > limitTime) {
        std::cerr << "c Elapsed time: " << elapsed << " seconds." << std::endl;
        std::cerr << "c Number of cubes generated: "
                  << queue.size() << '/' << options.numCubes << '\n';
        limitTime = elapsed + 10.0;
      }

      // ADAPTED: no task1/task2/MPI params; BCP done inline
      createCubes(formula, cubes, queue);
    }

    double elapsed = std::chrono::duration<double>(
        std::chrono::steady_clock::now() - wallStart).count();
    std::cerr << "c Time needed for generated the cubes: " << elapsed << '\n';

    delete m_propagator; m_propagator = nullptr;

    // collect the cubes (same as paper)
    while (!queue.empty()) {
      cubes.push_back(queue.top().lits);
      queue.pop();
    }

    delete m_branchingCubeHeuristic; m_branchingCubeHeuristic = nullptr;
    delete m_partialOrderHeuristic;  m_partialOrderHeuristic  = nullptr;
  }

  // Same interface as paper's getPartialOrderComputed — returns a level per var.
  // Here the heuristic is already deleted after generate(); call before that
  // or we cache the scores below.
  void getPartialOrderScores(std::vector<double>& scores, int nbVar) {
    scores.assign(nbVar + 1, 1.0);
    for (int i = 1; i <= nbVar; i++)
      scores[i] = m_cachedScores.size() > (unsigned)i ? m_cachedScores[i] : 1.0;
  }

  void cacheScores(const std::vector<double>& s) { m_cachedScores = s; }

 private:
  int  m_nbVariable = 0;
  bool m_verbose    = false;
  Propagator*             m_propagator             = nullptr;
  BranchingCubeHeuristic* m_branchingCubeHeuristic = nullptr;
  PartialOrderHeuristic*  m_partialOrderHeuristic  = nullptr;
  std::vector<double>     m_cachedScores;

  // ── initPropagator (same logic as paper) ─────────────────────────────
  void initPropagator(Formula* formula) {
    // Paper converts discount::Lit to bipe::Lit here; we skip that (same type)
    m_propagator = new Propagator(formula->getNbVar(), formula->getClauses());
  }

  // ── initQueue (same logic as paper) ──────────────────────────────────
  void initQueue(PriorityQueue& queue) {
    std::vector<Lit> units;
    // get the unit literals from the propagator (same comment as paper)
    m_propagator->propagate();
    Lit* trail = m_propagator->getTrail();
    for (int i = 0; i < m_propagator->getTrailSize(); i++)
      units.push_back(trail[i]);
    queue.push({units, (double)units.size()});
  }

  // ── createCubes ───────────────────────────────────────────────────────
  // Structurally identical to CubeGenerator::createCubes() in the paper.
  // What changed: TaskSolverSAT task1/task2 removed; instead of packing
  // into tasks and MPI_Isend/Irecv, we push BCP-extended cubes to the queue
  // directly.  Every other line (propagator fill, variable selection, trail
  // slicing, cancelUntilPos, pop) is unchanged.
  void createCubes(Formula* /*formula*/,
                   std::vector<std::vector<Lit>>& cubes,
                   PriorityQueue& queue) {
    const Cube& cBest = queue.top();

    // fill the propagator (same as paper)
    m_propagator->restart();
    for (auto& l : cBest.lits) {
      // "if (m_propagator->value(lb) > 1)" — only enqueue if unassigned
      if (m_propagator->value(l) > 1) m_propagator->uncheckedEnqueue(l);
    }
    if (!m_propagator->propagate() || m_propagator->getIsUnsat()) {
      queue.pop(); pruned++; return;  // should not normally happen
    }
    // Paper had: assert(cBest.lits.size() == m_propagator->getTrailSize());

    // select a variable to extend the cube (same as paper)
    Var v = m_branchingCubeHeuristic->next(cBest.lits, m_partialOrderHeuristic);

    // we cannot extend this cube, then go on with another one (same comment)
    if (v == var_Undef) {
      cubes.push_back(cBest.lits);
      queue.pop();
      return;
    }

    // Paper builds task1.lits / task2.lits and calls MPI_Isend/Irecv.
    // We instead push BCP-extended cubes directly to the queue.
    Lit* trail    = m_propagator->getTrail();
    int sizeTrail = m_propagator->getTrailSize();

    // Positive branch: cube ∪ {+v}, extended by BCP
    m_propagator->uncheckedEnqueue(Lit::makeLitTrue(v));
    if (m_propagator->propagate()) {
      // trail[0..trailSize] mirrors paper's
      //   "for (i = sizeTrail; i < trailSize; i++) task1.lits[i] = trail[i]"
      // but we take the full trail (includes original cube + propagated lits)
      std::vector<Lit> litPos(trail, trail + m_propagator->getTrailSize());
      queue.push({litPos, (double)litPos.size()});
    } else { pruned++; }  // BCP proved UNSAT — pruned (MPI would have got UNSAT)

    // Restore to pre-branch state (same as paper's cancelUntilPos(sizeTrail))
    m_propagator->cancelUntilPos(sizeTrail);

    // Negative branch: cube ∪ {-v}, extended by BCP
    m_propagator->uncheckedEnqueue(Lit::makeLitFalse(v));
    if (m_propagator->propagate()) {
      std::vector<Lit> litNeg(trail, trail + m_propagator->getTrailSize());
      queue.push({litNeg, (double)litNeg.size()});
    } else { pruned++; }

    // pop the element from the queue (same as paper)
    queue.pop();
  }
};

}  // namespace discount

// ─────────────────────────────────────────────────────────────────────────
//  JSON output
// ─────────────────────────────────────────────────────────────────────────

static std::string toJSON(
    const discount::Formula& f,
    const std::vector<double>& scores,
    const std::vector<int>& varOrder,
    const std::vector<std::vector<discount::Lit>>& cubes,
    int pruned, double elapsed, const std::string& strategy) {
  std::ostringstream j;
  j << "{\n";
  j << "  \"variable_order\": [";
  for (int i = 0; i < (int)varOrder.size(); i++) { if (i) j << ", "; j << varOrder[i]; }
  j << "],\n";
  j << "  \"variable_scores\": [0.0";
  for (int v = 1; v <= f.getNbVar(); v++) j << ", " << scores[v];
  j << "],\n";
  j << "  \"initial_cubes\": [\n";
  for (int i = 0; i < (int)cubes.size(); i++) {
    j << "    [";
    for (int k = 0; k < (int)cubes[i].size(); k++) { if (k) j << ", "; j << cubes[i][k].x; }
    j << "]"; if (i + 1 < (int)cubes.size()) j << ","; j << "\n";
  }
  j << "  ],\n";
  j << "  \"stats\": {\n"
    << "    \"num_vars\": "    << f.getNbVar()          << ",\n"
    << "    \"num_clauses\": " << f.getClauses().size() << ",\n"
    << "    \"num_cubes\": "   << cubes.size()          << ",\n"
    << "    \"pruned\": "      << pruned                << ",\n"
    << "    \"time_sec\": "    << elapsed               << ",\n"
    << "    \"strategy\": \""  << strategy              << "\"\n"
    << "  }\n}\n";
  return j.str();
}

// ─────────────────────────────────────────────────────────────────────────
//  main
// ─────────────────────────────────────────────────────────────────────────

int main(int argc, char** argv) {
  if (argc < 2) {
    std::cerr
      << "Usage: cube_gen <formula.cnf> [options]\n"
      << "  --num-cubes N   target cube count        (default: 64)\n"
      << "  --timeout   S   wall-clock budget (s)    (default: 30)\n"
      << "  --alpha     F   structural weight [0,1]  (default: 0.5)\n"
      << "  --strategy  S   combined|jwts|mindeg     (default: combined)\n"
      << "  --output    F   JSON file (default: stdout)\n"
      << "  --verbose       progress to stderr\n";
    return 1;
  }

  std::string cnfPath, outputFile, strategy = "combined";
  discount::OptionCubeGenerator opts;

  cnfPath = argv[1];
  for (int i = 2; i < argc; i++) {
    std::string arg = argv[i];
    auto nxt = [&]() -> std::string { return (++i < argc) ? argv[i] : ""; };
    if      (arg == "--num-cubes") opts.numCubes   = std::stoi(nxt());
    else if (arg == "--timeout")   opts.timeoutSec = std::stod(nxt());
    else if (arg == "--alpha")     opts.alpha       = std::stod(nxt());
    else if (arg == "--strategy")  strategy         = nxt();
    else if (arg == "--output")    outputFile       = nxt();
    else if (arg == "--verbose")   opts.verbose     = true;
    else { std::cerr << "Unknown option: " << arg << "\n"; return 1; }
  }
  opts.useMinDeg = (strategy != "jwts");

  discount::Formula formula;
  try { formula = discount::parseCNF(cnfPath); }
  catch (const std::exception& e) { std::cerr << "Error: " << e.what() << "\n"; return 1; }

  if (opts.verbose)
    std::cerr << "c Parsed " << formula.getNbVar() << " vars, "
              << formula.getClauses().size() << " clauses\n";

  auto wallStart = std::chrono::steady_clock::now();
  discount::CubeGenerator gen;
  std::vector<std::vector<discount::Lit>> cubes;

  // Cache partial order scores before generate() deletes the heuristic
  discount::PartialOrderHeuristic* po =
      discount::PartialOrderHeuristic::make(&formula, opts.useMinDeg);
  std::vector<double> structScores(formula.getNbVar() + 1, 1.0);
  for (int v = 1; v <= formula.getNbVar(); v++)
    structScores[v] = po->getLevel(v);
  delete po;

  gen.generate(&formula, cubes, opts);

  double elapsed = std::chrono::duration<double>(
      std::chrono::steady_clock::now() - wallStart).count();

  // Build combined scores: alpha*structural + (1-alpha)*JWTS
  std::vector<double> jwts(formula.getNbVar() + 1, 0.0);
  for (const auto& c : formula.getClauses()) {
    double w = std::pow(2.0, -(double)c.size());
    for (discount::Lit l : c) jwts[l.var()] += w;
  }
  auto norm = [](std::vector<double>& v) {
    double mx = *std::max_element(v.begin() + 1, v.end());
    if (mx > 0) for (size_t i = 1; i < v.size(); i++) v[i] /= mx;
  };
  norm(structScores); norm(jwts);

  std::vector<double> finalScores(formula.getNbVar() + 1, 0.0);
  for (int v = 1; v <= formula.getNbVar(); v++)
    finalScores[v] = opts.alpha * structScores[v] + (1.0 - opts.alpha) * jwts[v];

  // Variable order: best first
  std::vector<int> varOrder(formula.getNbVar());
  std::iota(varOrder.begin(), varOrder.end(), 1);
  std::sort(varOrder.begin(), varOrder.end(),
            [&](int a, int b) { return finalScores[a] > finalScores[b]; });

  std::string json = toJSON(formula, finalScores, varOrder, cubes,
                            gen.pruned, elapsed, strategy);
  if (outputFile.empty()) {
    std::cout << json;
  } else {
    std::ofstream out(outputFile);
    if (!out) { std::cerr << "Cannot write to " << outputFile << "\n"; return 1; }
    out << json;
  }
  return 0;
}
