#!/usr/bin/env python3
"""Reference implementation of the RepoAudit plugin protocol (v1).

Deliberately written in Python, not Go: docs/plugin-protocol.md's whole
point is that the protocol isn't Go-specific, and the only way to be
honest about that claim is to actually implement it in something else.

Detects one thing, on purpose kept trivial: a line containing the literal
string "TOTALLY_NOT_A_REAL_SECRET" (see docs/testing.md for why this
exact string, not a real credential pattern, is what the reference plugin
and its tests use).

Usage: reference-plugin.py [--misbehave=timeout|crash|fatal|error]
The --misbehave modes exist only to exercise RepoAudit's failure-handling
paths in tests; a real plugin has no reason to imitate them.
"""
import base64
import json
import sys
import time

MISBEHAVE = None
for arg in sys.argv[1:]:
    if arg.startswith("--misbehave="):
        MISBEHAVE = arg.split("=", 1)[1]


def send(msg):
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        msg = json.loads(line)

        if msg["type"] == "hello":
            if MISBEHAVE == "fatal":
                send({"type": "error", "fatal": True, "message": "reference-plugin: simulated fatal handshake error"})
                return
            send({
                "type": "hello_ack",
                "protocol_version": "1.0",
                "plugin_name": "reference-example",
                "plugin_version": "0.1.0",
            })
            continue

        if msg["type"] == "file":
            path = msg["path"]

            if MISBEHAVE == "crash":
                sys.exit(1)
            if MISBEHAVE == "timeout":
                time.sleep(30)  # longer than RepoAudit's 5s per-file timeout
            if MISBEHAVE == "error":
                send({"type": "error", "fatal": False, "path": path, "message": "reference-plugin: simulated non-fatal error"})
                continue

            content = base64.b64decode(msg["content"]).decode("utf-8", errors="replace")
            findings = []
            for i, text_line in enumerate(content.split("\n"), start=1):
                if "TOTALLY_NOT_A_REAL_SECRET" in text_line:
                    findings.append({
                        "id": "example_marker_found",
                        "severity": "MEDIUM",
                        "title": "Reference plugin test marker found",
                        "message": "This is the reference plugin's own test marker, not a real secret — it exists only to prove the plugin protocol round-trips a finding correctly.",
                        "fix": "Nothing to fix; this is a protocol test fixture.",
                        "line": i,
                    })
            send({"type": "result", "path": path, "findings": findings})
            continue


if __name__ == "__main__":
    main()
