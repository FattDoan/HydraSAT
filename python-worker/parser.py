import re


def parse_ganak_unweighted_count(stdout_text):
    """
    Parses GANAK2 unweight count output for the exact model count.
    Matches the line: "c o exact arb <count>"
    """

    # look for the "c o exact arb <number>" line.
    match = re.search(r"c\s+o\s+exact\s+arb\s+(\d+)", stdout_text)
    if match:
        return match.group(1)

    # fallback: Look for "c s exact arb int <number>"
    match = re.search(r"c\s+s\s+exact\s+arb\s+int\s+(\d+)", stdout_text)
    if match:
        return match.group(1)

    # 3. Last Resort: If it's UNSAT, the count is definitely 0
    if "s UNSATISFIABLE" in stdout_text:
        return "0"

    return "0"
