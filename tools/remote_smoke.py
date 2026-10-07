#!/usr/bin/env python3
"""Real-process interoperability smoke; dependencies are installed explicitly.
No network/dependency download is performed by this test itself.
"""
import argparse
import contextlib
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import tempfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "clients/python"))
import grpc
from remote.pb import service_pb2 as api, service_pb2_grpc as rpc
from client import query


@contextlib.contextmanager
def server(binary, snapshot, mode="locked", capacity=3, extra=()):
    command = [str(binary), "--snapshot", str(snapshot), "--listen=127.0.0.1:0", "--metrics-listen=", "--residency=" + mode, "--query-capacity=" + str(capacity), "--query-memory-budget=4294967296", *extra]
    process = subprocess.Popen(command, stderr=subprocess.PIPE, text=True)
    try:
        line = process.stderr.readline()
        if line.startswith("operational HTTP="):
            process.metrics_address = line.split("=",1)[1].strip()
            line = process.stderr.readline()
        if not line.startswith("serving gRPC="):
            raise RuntimeError("startup failure: " + line + process.stderr.read())
        address = line.split()[1].split("=", 1)[1]
        with grpc.insecure_channel(address, [("grpc.max_receive_message_length", 4 * 1024 * 1024 + 16)]) as channel:
            grpc.channel_ready_future(channel).result(timeout=10)
            yield process, address, rpc.GraphServiceStub(channel)
    finally:
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=40)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
            raise RuntimeError("shutdown watchdog expired")
        if process.returncode:
            raise RuntimeError("server exit: " + str(process.returncode) + " " + process.stderr.read())


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--java", action="store_true")
    a = p.parse_args()
    with server(ROOT / "bin/gophergraph-server", ROOT / "fixtures/snapshot_reference/topology.snapshot") as (_, address, stub):
        info = stub.GetServerInfo(api.ServerInfoRequest(), timeout=5)
        assert info.locked and info.warm_completed and info.residency_mode == "locked"
        assert len(info.snapshot_id) == 71
        installed = stub.ListWasmQueries(api.ListWasmQueriesRequest(), timeout=5)
        assert len(installed.queries) == 3
        for name in ("territory", "anti-territory", "between", "shared-targets"):
            result = query(stub, name, "A", "B", ["A", "B"], 10)
            assert result.ended
            print("python", name, result.counts, "bytes", result.bytes)
        subprocess.run([str(ROOT / "bin/gophergraph-remote-client"), "--insecure", "--address", address, "--node=A"], check=True)
        if a.java:
            host, port = address.rsplit(":", 1)
            subprocess.run(["mvn", "-o", "-s", str(ROOT / "clients/java/settings.xml"), "-Dmaven.repo.local=/tmp/gophergraph-m2", "-f", str(ROOT / "clients/java/pom.xml"), "-q", "exec:java", "-Dexec.args=" + host + " " + port + " A"], check=True)
    with tempfile.TemporaryDirectory(prefix="remote-empty-id-", dir=ROOT / ".measure") as directory:
        directory = Path(directory)
        (directory / "nodes").mkdir()
        (directory / "edges").mkdir()
        (directory / "nodes/data.csv").write_text('~id,~label\n"",EMPTY\nA,X\n')
        (directory / "edges/data.csv").write_text('~id,~from,~to,~label\n')
        subprocess.run([str(ROOT / "bin/gophergraph"), "build", "--nodes", str(directory / "nodes"), "--edges", str(directory / "edges"), "--output", str(directory / "empty.snapshot")], check=True, stdout=subprocess.DEVNULL)
        with server(ROOT / "bin/gophergraph-server", directory / "empty.snapshot") as (_, _, stub):
            result = query(stub, "territory", "", "", [], 10)
            assert result.header.query.HasField("node") and result.header.query.node == ""
            print("python empty external ID preserved")
    print("interoperability and process shutdown: PASS")


if __name__ == "__main__":
    main()
