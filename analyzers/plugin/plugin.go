// Package plugin implements RepoAudit's external plugin protocol (Phase 4):
// a subprocess speaking NDJSON on stdin/stdout, never a same-process Go
// plugin. See docs/plugin-protocol.md for the full contract this code
// implements — that document, not this one, is the source of truth for
// plugin authors.
package plugin

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"repoaudit/core"
)

const (
	protocolVersion = "1.0"

	// A per-file limit, not a whole-scan budget like githistory's — one
	// slow file just means this plugin stops being consulted for the rest
	// of the scan (see abandon), not that the scan waits longer overall.
	requestTimeout = 5 * time.Second

	// bufio.Scanner's default max token size (64KB) is far too small for
	// a base64-encoded file near core.MaxFileSize (2 MiB -> ~2.8MB
	// base64, plus JSON envelope overhead).
	maxLineSize = 8 << 20 // 8 MiB
)

var errTimeout = errors.New("plugin did not respond in time")

// Plugin wraps one subprocess and implements core.Analyzer, so it plugs
// into core.Scanner exactly like secrets/docker/cicd do — the scan loop
// doesn't need to know an analyzer is backed by a pipe instead of Go code.
type Plugin struct {
	name string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Scanner

	mu    sync.Mutex
	alive bool
}

// Load starts execPath, performs the handshake, and returns a ready-to-use
// Plugin. The handshake happens once, up front, so a version mismatch or a
// broken plugin executable is reported immediately at scan start rather
// than confusingly on the first file.
func Load(execPath string) (*Plugin, error) {
	cmd := exec.Command(execPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %s: could not open stdin pipe: %w", execPath, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %s: could not open stdout pipe: %w", execPath, err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin %s: could not start: %w", execPath, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	p := &Plugin{cmd: cmd, in: stdin, out: scanner, alive: true}
	if err := p.handshake(); err != nil {
		_ = p.cmd.Process.Kill()
		return nil, fmt.Errorf("plugin %s: handshake failed: %w", execPath, err)
	}
	return p, nil
}

func (p *Plugin) Name() string { return p.name }

// Close signals the plugin to exit (closing its stdin) and waits for it.
// Safe to call on an already-abandoned plugin.
func (p *Plugin) Close() {
	p.mu.Lock()
	wasAlive := p.alive
	p.alive = false
	p.mu.Unlock()
	if !wasAlive {
		return
	}
	_ = p.in.Close()
	_ = p.cmd.Wait()
}

func (p *Plugin) handshake() error {
	req := map[string]any{
		"type":              "hello",
		"protocol_version":  protocolVersion,
		"repoaudit_version": "dev",
	}
	if err := p.writeLine(req); err != nil {
		return fmt.Errorf("sending hello: %w", err)
	}

	line, err := p.readLine(requestTimeout)
	if err != nil {
		return fmt.Errorf("no response to hello: %w", err)
	}

	var msg incomingMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		return fmt.Errorf("invalid response to hello: %w", err)
	}

	switch msg.Type {
	case "hello_ack":
		if msg.PluginName == "" {
			return errors.New("hello_ack missing plugin_name")
		}
		p.name = msg.PluginName
		return nil
	case "error":
		return fmt.Errorf("plugin rejected handshake: %s", msg.Message)
	default:
		return fmt.Errorf("unexpected message type %q during handshake", msg.Type)
	}
}

// Run sends one file over the pipe and waits for exactly one result,
// per docs/plugin-protocol.md. Any failure — a malformed response, a
// timeout, or the process dying mid-request — abandons the plugin for the
// rest of the scan rather than retrying: see abandon.
func (p *Plugin) Run(file core.FileContext) []core.Finding {
	p.mu.Lock()
	alive := p.alive
	p.mu.Unlock()
	if !alive {
		return nil
	}

	req := map[string]any{
		"type":    "file",
		"path":    file.Path,
		"content": base64.StdEncoding.EncodeToString(file.Content),
	}
	if err := p.writeLine(req); err != nil {
		p.abandon(fmt.Sprintf("plugin %q: failed to send %s (%v) — skipping it for the rest of the scan", p.name, file.Path, err))
		return nil
	}

	line, err := p.readLine(requestTimeout)
	if err != nil {
		if errors.Is(err, errTimeout) {
			p.abandon(fmt.Sprintf("plugin %q did not respond within %s on %s — skipping it for the rest of the scan", p.name, requestTimeout, file.Path))
		} else {
			p.abandon(fmt.Sprintf("plugin %q crashed or closed its connection while processing %s (%v) — skipping it for the rest of the scan", p.name, file.Path, err))
		}
		return nil
	}

	var msg incomingMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		p.abandon(fmt.Sprintf("plugin %q sent invalid JSON while processing %s — skipping it for the rest of the scan", p.name, file.Path))
		return nil
	}

	switch msg.Type {
	case "result":
		if msg.Path != file.Path {
			p.abandon(fmt.Sprintf("plugin %q returned a result for %q instead of the requested %q — skipping it for the rest of the scan", p.name, msg.Path, file.Path))
			return nil
		}
		return p.convertFindings(msg.Findings, file.Path)
	case "error":
		if msg.Fatal {
			p.abandon(fmt.Sprintf("plugin %q reported a fatal error: %s", p.name, msg.Message))
		} else {
			fmt.Fprintf(os.Stderr, "⚠️  plugin %q failed on %s: %s\n", p.name, file.Path, msg.Message)
		}
		return nil
	default:
		p.abandon(fmt.Sprintf("plugin %q sent an unexpected message type %q — skipping it for the rest of the scan", p.name, msg.Type))
		return nil
	}
}

// abandon is the single funnel for every plugin failure mode: an explicit
// fatal error, a timeout, or the process dying outright all end up here.
// A dead process makes the next readLine fail (EOF or a read error) via
// the exact same path a timeout takes, so there's no separate "the
// process crashed" branch to maintain — it falls out of treating stdout
// closing unexpectedly as just another read failure.
func (p *Plugin) abandon(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.alive {
		return
	}
	p.alive = false
	fmt.Fprintf(os.Stderr, "⚠️  %s\n", reason)
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *Plugin) writeLine(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = p.in.Write(b)
	return err
}

// readLine reads one NDJSON line with a timeout. bufio.Scanner has no
// built-in deadline, and relying on the concrete pipe type supporting
// SetReadDeadline isn't guaranteed across platforms, so this uses the
// standard goroutine+select timeout idiom instead. If the timeout fires,
// the scan goroutine is still blocked on the underlying read; it unblocks
// (and its buffered, now-unread send becomes a no-op) once abandon kills
// the process — no permanent goroutine leak as long as every timeout path
// leads to abandon, which Run and handshake both guarantee.
func (p *Plugin) readLine(timeout time.Duration) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		if p.out.Scan() {
			b := make([]byte, len(p.out.Bytes()))
			copy(b, p.out.Bytes())
			ch <- result{line: b}
			return
		}
		err := p.out.Err()
		if err == nil {
			err = io.EOF
		}
		ch <- result{err: err}
	}()

	select {
	case res := <-ch:
		return res.line, res.err
	case <-time.After(timeout):
		return nil, errTimeout
	}
}

type wireFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
	Line     int    `json:"line,omitempty"`
	Context  string `json:"context,omitempty"`
	Category string `json:"category,omitempty"`
}

type incomingMsg struct {
	Type          string        `json:"type"`
	PluginName    string        `json:"plugin_name,omitempty"`
	PluginVersion string        `json:"plugin_version,omitempty"`
	Path          string        `json:"path,omitempty"`
	Findings      []wireFinding `json:"findings,omitempty"`
	Fatal         bool          `json:"fatal,omitempty"`
	Message       string        `json:"message,omitempty"`
}

var validSeverity = map[string]core.Severity{
	"CRITICAL": core.Critical,
	"HIGH":     core.High,
	"MEDIUM":   core.Medium,
	"LOW":      core.Low,
}

// convertFindings applies the same non-negotiable quality bar the
// repoaudit-finding skill imposes on every built-in rule — a finding with
// no message/fix, or a severity string that isn't one of the four exact
// values, is dropped and logged, not silently accepted or guessed at.
func (p *Plugin) convertFindings(wire []wireFinding, path string) []core.Finding {
	var findings []core.Finding
	for _, wf := range wire {
		sev, ok := validSeverity[wf.Severity]
		if !ok {
			fmt.Fprintf(os.Stderr, "⚠️  plugin %q: dropped a finding on %s with invalid severity %q\n", p.name, path, wf.Severity)
			continue
		}
		if wf.Message == "" || wf.Fix == "" {
			fmt.Fprintf(os.Stderr, "⚠️  plugin %q: dropped a finding on %s with an empty message or fix\n", p.name, path)
			continue
		}

		id := wf.ID
		prefix := p.name + "."
		if !strings.HasPrefix(id, prefix) {
			id = prefix + id
		}
		category := wf.Category
		if category == "" {
			category = p.name
		}

		findings = append(findings, core.Finding{
			ID:       id,
			Severity: sev,
			Title:    wf.Title,
			Message:  wf.Message,
			Fix:      wf.Fix,
			File:     path,
			Line:     wf.Line,
			Category: category,
			Context:  wf.Context,
		})
	}
	return findings
}
