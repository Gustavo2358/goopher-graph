#!/usr/bin/env python3
"""Reduce conflict_io_measure JSONL, checking output identity before comparison."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import statistics


def summarize(paths):
    groups = defaultdict(lambda: defaultdict(list))
    identities = {}
    for path in paths:
        for line in path.read_text().splitlines():
            s = json.loads(line)
            report = dict(s["Report"])
            report.pop("Times")
            identity = s["snapshot_sha256"], report, s["DiagnosticCount"], s["DiagnosticSHA256"]
            key = s["profile"], s["policy"]
            if key in identities and identities[key] != identity:
                raise ValueError(f"output mismatch for {key}")
            identities[key] = identity
            groups[(s["profile"], s["policy"], s["budget"])][s["variant"]].append(s)
    result = []
    fields = {"build_s": lambda s: s["Build"] / 1e9,
              "ingest_merge_s": lambda s: s["Report"]["Times"]["IngestMerge"] / 1e9,
              "canonicalize_s": lambda s: s["Report"]["Times"]["Canonicalize"] / 1e9,
              "publication_s": lambda s: s["Write"] / 1e9,
              "read_bytes": lambda s: s["Scratch"]["Read"],
              "written_bytes": lambda s: s["Scratch"]["Written"],
              "io_bytes": lambda s: s["Scratch"]["Read"] + s["Scratch"]["Written"],
              "peak_scratch_bytes": lambda s: s["Scratch"]["Peak"],
              "rss_kib": lambda s: s["rss_kib"]}
    for key, variants in sorted(groups.items()):
        a, b = variants["baseline"], variants["candidate"]
        if len(a) != len(b) or {s["repeat"] for s in a} != {s["repeat"] for s in b}:
            raise ValueError(f"unpaired samples for {key}")
        row = dict(profile=key[0], policy=key[1], budget=key[2], repeats=len(a))
        for name, get in fields.items():
            left, right = [get(s) for s in a], [get(s) for s in b]
            am, bm = statistics.median(left), statistics.median(right)
            row[name] = {"baseline_median": am, "candidate_median": bm,
                         "delta_percent": (bm / am - 1) * 100 if am else 0,
                         "baseline_range": [min(left), max(left)],
                         "candidate_range": [min(right), max(right)]}
        pairs = {s["repeat"]: s for s in a}
        row["paired_build_delta_percent"] = [(s["Build"] / pairs[s["repeat"]]["Build"] - 1) * 100
                                             for s in sorted(b, key=lambda s: s["repeat"])]
        row["snapshot_sha256"] = a[0]["snapshot_sha256"]
        row["diagnostic_count"] = a[0]["DiagnosticCount"]
        result.append(row)
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("samples", type=Path, nargs="+")
    args = parser.parse_args()
    print(json.dumps(summarize(args.samples), indent=2))
