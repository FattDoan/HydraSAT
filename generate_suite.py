import os
import networkx as nx
import cnfgen

def main():
    out_dir = "cnf_instances"
    os.makedirs(out_dir, exist_ok=True)
    print(f"Generating #SAT benchmark suite in '{out_dir}/'...")
    # RANDOM 3-SAT (Mixed SAT/UNSAT)
    # ==========================================
    # For #SAT, 70-100 variables is often a sweet spot for testing.
    # Ratio (M/N) = 3.0: Under-constrained, tons of models.
    # Ratio (M/N) = 4.26: Phase transition, very hard to find the few models.
    var_counts = [120, 140, 150, 155, 160, 165, 170]
    ratios = [3.0, 4.2] 
    
    for n in var_counts:
        for r in ratios:
            m = int(n * r)
            print(f"Generating Random 3-SAT: {n} vars, {m} clauses (ratio {r})...")
            # Using a fixed seed for reproducibility for your report!
            F = cnfgen.RandomKCNF(3, n, m, seed=42+n) 
            F.to_file(os.path.join(out_dir, f"rand3sat_{n}v_{m}c.cnf"))


    var_counts_4sat = [80, 90]
    # RANDOM 4-SAT (Mixed SAT/UNSAT)
    for n in var_counts_4sat:
        for r in ratios:
            m = int(n * r)
            print(f"Generating Random 4-SAT: {n} vars, {m} clauses (ratio {r})...")
            F = cnfgen.RandomKCNF(4, n, m, seed=100+n) 
            F.to_file(os.path.join(out_dir, f"rand4sat_{n}v_{m}c.cnf"))


    print("\nDone! All files generated.")

if __name__ == '__main__':
    main()
