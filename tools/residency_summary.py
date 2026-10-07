#!/usr/bin/env python3
"""Validate state/count invariants and summarize residencymeasure JSONL."""
import collections
import json
import statistics
import sys


def summarize(path):
    with open(path) as file:
        rows = [json.loads(line) for line in file]
    groups = collections.defaultdict(list)
    trials = [r for r in rows if r["type"] == "trial"]
    for r in trials:
        p = r["before"]
        assert p["Pages"] > 0
        assert p["ResidentPages"] == (0 if r["condition"] == "cold" else p["Pages"]), r
        assert r["query_phase"]["Major"] == 0 or r["condition"] == "cold", r
        if r["condition"] == "cold":
            assert r["query_phase"]["Major"] > 0, r
        groups[r["query"], r["condition"]].append(r)
    sizes = {len(v) for v in groups.values()}
    assert len(sizes) == 1 and sizes != {0}, sizes
    result = []
    for (query, condition), values in sorted(groups.items()):
        counts = {(r["nodes"], r["edges"]) for r in trials if r["query"] == query}
        assert len(counts) == 1, counts
        times = sorted(r["query_phase"]["Millis"] for r in values)
        encoded = [r for r in values if r["encode_phase"] is not None]
        if encoded:
            assert len({r["payload_bytes"] for r in encoded}) == 1
        result.append(dict(query=query, condition=condition, samples=len(values),
            nodes=values[0]["nodes"], edges=values[0]["edges"],
            median_ms=statistics.median(times), p95_ms=times[int((len(times)-1)*.95)],
            min_ms=times[0], max_ms=times[-1],
            cpu_median_ms=statistics.median(r["query_phase"]["CPUMillis"] for r in values),
            minor_median=statistics.median(r["query_phase"]["Minor"] for r in values),
            major_median=statistics.median(r["query_phase"]["Major"] for r in values),
            encode_samples=len(encoded), encode_median_ms=statistics.median(r["encode_phase"]["Millis"] for r in encoded) if encoded else None,
            query_encode_median_ms=statistics.median(r["query_phase"]["Millis"]+r["encode_phase"]["Millis"] for r in encoded) if encoded else None,
            query_alloc_median=statistics.median(r["query_alloc_bytes"] for r in values)))
    return dict(startup=[r for r in rows if r["type"] == "startup"],
                unsupported=[r for r in rows if r["type"] == "unsupported"],
                groups=result)


if __name__ == "__main__":
    print(json.dumps({path: summarize(path) for path in sys.argv[1:]}, indent=2))
