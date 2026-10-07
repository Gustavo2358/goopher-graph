#!/usr/bin/env python3
"""Compare two prebuilt spill implementations on deterministic CSV corpora.

Scratch must be on disk. Counts are logical bytes at the scratch port, not
physical device traffic. Runs are sequential and alternate implementation order.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def generate(root, n):
    manifests = {}
    for profile in ("sets-edges", "mixed", "single"):
        corpus = root / profile
        for role in ("nodes", "edges"):
            (corpus / role).mkdir(parents=True, exist_ok=True)
        with (corpus / "nodes/data.csv").open("w") as f:
            if profile == "single":
                f.write("~id,~label,group:String(single),rank:Int(single)\n")
                for repeat in range(3):
                    for i in range(n):
                        f.write(f"n{i:09d},TYPE;GROUP,g{(i + repeat % 2) % 10},{i + repeat % 2}\n")
            else:
                rank = "rank:Int(single)" if profile == "mixed" else "rank:Int"
                f.write(f"~id,~label,group:String,{rank}\n")
                for i in range(n):
                    f.write(f"n{i:09d},TYPE;GROUP,g{i % 10},{i}\n")
                if profile == "mixed":
                    for i in range(n):
                        f.write(f"n{i:09d},TYPE;GROUP,,{i + 1}\n")
        with (corpus / "edges/data.csv").open("w") as f:
            f.write("~id,~from,~to,~label,score:Int\n")
            if profile != "single":
                for i in range(n):
                    base, end = (0, n * 9 // 10) if i < n * 9 // 10 else (n * 9 // 10, n)
                    length = end - base
                    destinations = (base + (i - base + 1) % length,
                                    base + (i - base + 2) % length, base, i, i)
                    sources = (i, i, i, base, i)
                    for k in range(5):
                        label = "L" if k < 3 else ("M" if k == 3 else "U")
                        f.write(f"e{5 * i + k:09d},n{sources[k]:09d},n{destinations[k]:09d},{label},{i % 100}\n")
        manifests[profile] = {
            "nodes": n, "edges": 0 if profile == "single" else 5 * n,
            "records": (3 if profile == "single" else 7 if profile == "mixed" else 6) * n,
            "property_contributions": (6 if profile == "single" else 8 if profile == "mixed" else 7) * n,
            "ordered_contributions_last_first": (6 if profile == "single" else 2 if profile == "mixed" else 0) * n,
            "input_bytes": sum(p.stat().st_size for p in corpus.glob("*/data.csv")),
        }
    return manifests


def sha256(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def measure(args):
    root = args.root.resolve()
    root.mkdir(parents=True, exist_ok=True)
    scratch = root / "scratch"
    scratch.mkdir(exist_ok=True)
    manifest = generate(root, args.nodes)
    metadata = {"corpora": manifest, "baseline_sha256": sha256(args.baseline),
                "candidate_sha256": sha256(args.candidate), "nodes": args.nodes,
                "repeats": args.repeats, "budgets": args.budgets, "policies": args.policies}
    (root / "manifest.json").write_text(json.dumps(metadata, indent=2) + "\n")
    reference = {}
    with (root / "samples.jsonl").open("w") as raw:
        for budget in args.budgets:
            for profile in args.profiles:
                for policy in args.policies:
                    for repeat in range(args.repeats):
                        order = ("baseline", "candidate") if repeat % 2 == 0 else ("candidate", "baseline")
                        for variant in order:
                            label = f"{profile}-{policy}-{budget}-{repeat}-{variant}"
                            usage_file = root / f"{label}.usage.json"
                            cmd = ["/usr/bin/time", "-o", str(usage_file), "-f",
                                   '{"rss_kib":%M,"user_s":%U,"system_s":%S,"major_faults":%F}',
                                   str(getattr(args, variant).resolve()), "--input", str(root / profile),
                                   "--temp-dir", str(scratch), "--memory-budget", str(budget),
                                   "--node-property-conflict", policy]
                            result = subprocess.run(cmd, text=True, capture_output=True, check=True)
                            sample = json.loads(result.stdout)
                            sample.update(json.loads(usage_file.read_text()))
                            sample.update(profile=profile, policy=policy, budget=budget,
                                          repeat=repeat, variant=variant)
                            sample["snapshot_sha256"] = sha256(root / profile / "graph.snapshot")
                            report = dict(sample["Report"])
                            report.pop("Times")
                            identity = (sample["snapshot_sha256"], report,
                                        sample["DiagnosticCount"], sample["DiagnosticSHA256"])
                            key = profile, policy
                            if key in reference and reference[key] != identity:
                                raise RuntimeError(f"snapshot/report/diagnostic mismatch: {label}")
                            reference[key] = identity
                            if any(scratch.iterdir()):
                                raise RuntimeError(f"scratch leak: {label}")
                            raw.write(json.dumps(sample) + "\n")
                            raw.flush()
                            print(json.dumps({"sample": label, "build_s": sample["Build"] / 1e9,
                                              "read_gib": sample["Scratch"]["Read"] / 2**30,
                                              "written_gib": sample["Scratch"]["Written"] / 2**30}), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, required=True)
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--nodes", type=int, default=400000)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--budgets", type=int, nargs="+", default=[16 << 20])
    parser.add_argument("--profiles", nargs="+", choices=("sets-edges", "mixed", "single"),
                        default=["sets-edges", "mixed", "single"])
    parser.add_argument("--policies", nargs="+", choices=("last-wins", "first-wins", "drop"), default=["last-wins"])
    args = parser.parse_args()
    if args.nodes < 10 or args.nodes > 1000000 or args.repeats < 1 or min(args.budgets) < 1 << 20:
        parser.error("nodes: 10..1000000; repeats: >=1; budgets: >=1 MiB")
    measure(args)
