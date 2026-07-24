# RepoScan plugin protocol (v1)

This document is the contract for writing an external RepoScan plugin. It's aimed at someone who has never read RepoScan's Go source — everything a plugin author needs is here.

## Why a separate process, not a Go plugin

RepoScan plugins run as a **separate process**, communicating over stdin/stdout. Not a native Go plugin (`plugin` package), and this isn't a stopgap — it's a permanent decision, for two independent reasons:

- **Security**: a Go interface is a compile-time contract, not a runtime sandbox. Nothing stops code compiled into the same binary from ignoring the interface and reading arbitrary files, making network calls, or worse, regardless of what the interface's parameters look like. Only process isolation makes that a real, enforced boundary rather than a polite convention.
- **Stability**: a panic or infinite loop in a same-process plugin takes down all of RepoScan — there's no way to forcibly stop a stuck goroutine without killing the whole process. A subprocess that crashes or hangs is just an error the host can detect and recover from.

Go's native `plugin` package is also a poor fit on its own terms: no Windows support, and it requires the plugin to be compiled with the exact same Go toolchain version as the host.

WASM (WASI) would give stronger sandboxing than a subprocess — no filesystem or network access at all unless explicitly granted — and is a plausible future upgrade if a real need for tighter sandboxing shows up. Not built now: a subprocess with a byte-only wire protocol (see below) is simpler to implement in any language, and already prevents the capability leaks that matter for this protocol's scope.

## Process lifecycle

1. RepoScan starts your plugin executable once per scan (not once per file — spawning a process per file would be far too slow on a large repo).
2. RepoScan sends a `hello` message on your stdin. You reply with `hello_ack` on your stdout.
3. For each eligible file in the repo (already filtered by RepoScan's own size/binary/vendored-path rules — you never see files larger than 2 MiB, binary files, or anything under `vendor/`/`node_modules/`), RepoScan sends one `file` message and waits for exactly one `result` message back before sending the next file. One line of JSON per message (NDJSON), synchronous, no concurrent requests.
4. When there are no more files, RepoScan closes your stdin. Exit when you see EOF.

## Byte-only contract — read this before writing any file I/O in your plugin

**A `file` message gives you the file's content directly (base64-encoded). It never gives you a path you're expected to open yourself.** The `path` field is metadata for your own findings' context (e.g. to decide "this looks like a test file") — it is not a filesystem location you can resolve, because RepoScan gives no guarantee your process's working directory has any relationship to the scanned repo. If your plugin calls its own file-reading code instead of using the `content` field it was sent, it will not work reliably, and it defeats the entire point of the sandboxing this protocol provides.

Your plugin gets no filesystem access, no network access, and no elevated capability of any kind beyond the bytes it's handed. If a future plugin genuinely needs more (e.g. resolving imports across files), that's an explicit protocol extension to negotiate later — never an ambient default.

## Messages

Every message is one JSON object per line on stdin/stdout (NDJSON). All messages have a `type` field.

### `hello` (host → plugin, once)

```json
{"type": "hello", "protocol_version": "1.0", "reposcan_version": "0.3.0"}
```

### `hello_ack` (plugin → host, once)

```json
{"type": "hello_ack", "protocol_version": "1.0", "plugin_name": "terraform-lint", "plugin_version": "0.1.0"}
```

`plugin_name` must be a short, stable identifier (lowercase, no spaces — treat it like a package name). It's used to namespace your finding IDs: see below.

If you don't support the given `protocol_version`, reply with a fatal error instead (see below) rather than guessing.

### `file` (host → plugin, once per eligible file)

```json
{"type": "file", "path": "modules/network/main.tf", "content": "<base64>"}
```

### `result` (plugin → host, exactly one per `file` message, in order)

```json
{
  "type": "result",
  "path": "modules/network/main.tf",
  "findings": [
    {
      "id": "terraform.public_s3_bucket",
      "severity": "HIGH",
      "title": "S3 bucket publicly readable",
      "message": "This bucket's ACL grants public-read access. Anyone on the internet can list and download its contents.",
      "fix": "Remove the public-read ACL and use bucket policies scoped to specific principals instead.",
      "line": 14,
      "context": ""
    }
  ]
}
```

Echo back the same `path` you were given — RepoScan uses it to double check the response matches the request it's replying to. If a file produces no findings, reply with `"findings": []`, not a missing/omitted response.

**Finding fields:**

| Field | Required | Notes |
|---|---|---|
| `id` | yes | Stable rule identifier, e.g. `"terraform.public_s3_bucket"`. Should start with `<plugin_name>.` — if it doesn't, RepoScan prefixes it for you automatically, as a collision guard against another plugin (or a built-in analyzer) using the same bare ID. |
| `severity` | yes | One of `"CRITICAL"`, `"HIGH"`, `"MEDIUM"`, `"LOW"` — exact uppercase strings, no others accepted. |
| `title` | yes | One-line summary. |
| `message` | yes, non-empty | *Why* this is dangerous — not just that a pattern matched. A finding with an empty message is dropped and logged as a plugin bug, not silently accepted. |
| `fix` | yes, non-empty | A concrete, actionable fix — same non-negotiable rule as every built-in RepoScan rule (see `docs/decisions/` for why). |
| `line` | no | Defaults to `0` (not applicable) if omitted. |
| `context` | no | Optional triage hint, never a severity signal — defaults to `""` if omitted. |

There is no `file` field on a finding (RepoScan already knows which file it asked about) and no `category` field required — if omitted, RepoScan uses your `plugin_name` as the category. There is no `commit_hash` field — plugins don't see git history in this version of the protocol.

Unknown fields on any message are ignored, not rejected — this schema is expected to grow. Don't rely on unknown-field-rejection as validation; that's not how this protocol behaves, deliberately, so old plugins keep working against newer RepoScan versions where reasonable.

### `error` (plugin → host, any time)

```json
{"type": "error", "fatal": false, "path": "modules/network/main.tf", "message": "failed to parse HCL: unexpected token at line 3"}
```

`fatal: false` (with a `path`) means this one file couldn't be processed — RepoScan logs it and moves on to the next file; the rest of the scan, and the rest of your plugin's findings, are unaffected.

`fatal: true` (no `path` needed) means your plugin can't continue at all (e.g. the handshake's `protocol_version` is one you don't support). RepoScan abandons your plugin for the remainder of the scan — see below.

## Failure handling: a broken plugin degrades, it never fails the scan

Three ways a plugin can fail, all handled identically:

1. It sends `{"type": "error", "fatal": true, ...}`.
2. It doesn't respond to a `file` message within **5 seconds** (a per-file timeout, not a whole-scan budget — there's no equivalent of the git-history analyzer's time-budget-with-partial-coverage model here; one slow file just means that plugin stops being consulted for the rest of the scan).
3. Its process dies (crashes, is killed, exits unexpectedly) — detected the same way a timeout is: the read from its stdout fails or returns EOF where a response was expected.

In all three cases: RepoScan prints one warning naming the plugin and the reason, kills the process if it's still alive, and continues the scan without that plugin's findings for every remaining file. It does not retry, restart, or re-invoke a plugin that has failed once — a plugin that times out or crashes on one file has a real problem that will very likely recur, and retrying adds latency for no expected benefit. This mirrors how RepoScan already treats other failures it can't fully control (a slow/unreachable OSV.dev API in the Dependency Scanner, a git-history scan that runs out of its time budget): degrade with a clear warning, never let one failing check take down the whole scan.

## Writing a plugin in a language other than Go

Nothing here is Go-specific. A plugin is any executable that speaks this NDJSON protocol on stdin/stdout — a Python script, a shell script piping into `jq`, a compiled binary in any language. Reading one line, decoding JSON, deciding, encoding JSON, writing one line back, in a loop, is the entire implementation surface.
