// Package docker implements RepoScan's Phase 2 Dockerfile detection rules:
// running as root, floating tags, and ADD used where COPY would do.
// Secrets hardcoded in ENV/ARG instructions need no rule here — a
// Dockerfile is just text, so analyzers/secrets already scans it via the
// same core.Scanner pass, tagged Category "secrets".
package docker

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xchebila/reposcan/core"
)

type Analyzer struct{}

func New() *Analyzer { return &Analyzer{} }

func (a *Analyzer) Name() string { return "docker" }

func (a *Analyzer) Run(file core.FileContext) []core.Finding {
	if !isDockerfile(file.Path) {
		return nil
	}

	lines := strings.Split(string(file.Content), "\n")

	var findings []core.Finding
	findings = append(findings, checkTags(file.Path, lines)...)
	findings = append(findings, checkAdd(file.Path, lines)...)
	findings = append(findings, checkUser(file.Path, lines)...)
	return findings
}

func isDockerfile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, "dockerfile") || strings.HasSuffix(base, ".dockerfile")
}

var fromPattern = regexp.MustCompile(`(?i)^\s*FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+[Aa][Ss]\s+(\S+))?\s*$`)

// checkTags flags images pulled without a pinned tag or digest — "latest"
// (implicit or explicit) means the same Dockerfile can build a different
// image tomorrow, with no record of what changed.
//
// False positive this rule must not trip on: multi-stage builds referencing
// an earlier stage by name (FROM builder), which look exactly like an
// untagged image pull but aren't one. Collecting every stage name declared
// via "AS <name>" first, and skipping any FROM whose image matches one, is
// what avoids that.
func checkTags(path string, lines []string) []core.Finding {
	stageNames := map[string]bool{}
	for _, line := range lines {
		if m := fromPattern.FindStringSubmatch(line); m != nil && m[2] != "" {
			stageNames[m[2]] = true
		}
	}

	var findings []core.Finding
	for i, line := range lines {
		m := fromPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		image := m[1]
		if stageNames[image] || strings.EqualFold(image, "scratch") {
			continue
		}
		if strings.Contains(image, "@") {
			continue // digest-pinned, the strongest form of "not latest"
		}

		lastSegment := image
		if idx := strings.LastIndex(image, "/"); idx != -1 {
			lastSegment = image[idx+1:]
		}
		tagIdx := strings.Index(lastSegment, ":")

		switch {
		case tagIdx == -1:
			findings = append(findings, finding(latestTagRule, path, i+1))
		case lastSegment[tagIdx+1:] == "latest":
			findings = append(findings, finding(latestTagRule, path, i+1))
		}
	}
	return findings
}

var addPattern = regexp.MustCompile(`(?i)^\s*ADD\s+(?:--\S+\s+)*(\S+)`)
var archiveExt = regexp.MustCompile(`(?i)\.(tar(\.(gz|bz2|xz))?|tgz|zip)$`)

// checkAdd flags ADD used for plain local files/directories, where COPY is
// the more predictable, more transparent choice (Docker's own recommended
// practice). ADD's two genuinely unique features — fetching a URL, and
// auto-extracting a recognized archive — are exactly what's excluded here,
// since those aren't replaceable by COPY at all.
//
// False positive this rule must not trip on: `ADD app.tar.gz /app` for its
// auto-extraction behavior is legitimate ADD usage with no COPY equivalent;
// excluded via the archive-extension check.
func checkAdd(path string, lines []string) []core.Finding {
	var findings []core.Finding
	for i, line := range lines {
		m := addPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		src := m[1]
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			continue
		}
		if archiveExt.MatchString(src) {
			continue
		}
		findings = append(findings, finding(addInsteadOfCopyRule, path, i+1))
	}
	return findings
}

var userPattern = regexp.MustCompile(`(?i)^\s*USER\s+(\S+)`)

// checkUser looks only at the last build stage, since only the final
// stage's USER instruction affects the resulting image — an earlier stage
// running as root to install packages, then switching to a non-root USER
// before the final stage, is a normal and safe pattern that must not be
// flagged.
//
// False positive this rule must not trip on: a base image that already
// sets a non-root USER internally (e.g. official node images end in `USER
// node`). A Dockerfile that adds nothing further inherits that non-root
// user at runtime but looks, from this file alone, identical to one that
// inherits root — inspecting the base image's own effective USER is out of
// scope (no image pulling, no deep static analysis, per vision.md).
//
// One specific case of that is worth a real exception, not just a
// disclosed limitation: distroless images (gcr.io/distroless/*) put
// "nonroot" directly in the tag as a documented convention (e.g.
// static-debian12:nonroot-amd64) — confirmed against prometheus's own
// Dockerfile.distroless, which relies on exactly this with a comment
// saying so and no USER instruction of its own.
func checkUser(path string, lines []string) []core.Finding {
	lastStage := lines
	lastStageImage := ""
	for i, line := range lines {
		if m := fromPattern.FindStringSubmatch(line); m != nil {
			lastStageImage = m[1]
			if i > 0 {
				lastStage = lines[i:]
			}
		}
	}

	lastUser := ""
	lastUserLine := 0
	for i, line := range lastStage {
		if m := userPattern.FindStringSubmatch(line); m != nil {
			lastUser = m[1]
			lastUserLine = i
		}
	}

	offset := len(lines) - len(lastStage)
	if lastUser == "" {
		if strings.Contains(strings.ToLower(lastStageImage), "nonroot") {
			return nil
		}
		return []core.Finding{finding(noUserRule, path, offset+1)}
	}

	normalized := lastUser
	if idx := strings.Index(normalized, ":"); idx != -1 {
		normalized = normalized[:idx]
	}
	if normalized == "root" || normalized == "0" {
		return []core.Finding{finding(userRootRule, path, offset+lastUserLine+1)}
	}
	return nil
}

type rule struct {
	id       string
	severity core.Severity
	title    string
	message  string
	fix      string
}

var (
	latestTagRule = rule{
		id:       "docker.latest_tag",
		severity: core.Low,
		title:    "Image pulled without a pinned tag",
		message:  "This FROM instruction has no tag (defaults to \"latest\") or is pinned to \"latest\" explicitly. The same Dockerfile can produce a different image tomorrow with no record of what changed.",
		fix:      "Pin an explicit version tag or, better, a digest (FROM image@sha256:...) for reproducible builds.",
	}
	addInsteadOfCopyRule = rule{
		id:       "docker.add_instead_of_copy",
		severity: core.Low,
		title:    "ADD used to copy local files",
		message:  "ADD copying a local path behaves like COPY but adds implicit tar-extraction and remote-URL-fetch semantics that make the build step less predictable and harder to audit at a glance.",
		fix:      "Use COPY for local files and directories; keep ADD only for its unique features (URL fetch, archive auto-extraction).",
	}
	noUserRule = rule{
		id:       "docker.no_nonroot_user",
		severity: core.Medium,
		title:    "No non-root USER set",
		message:  "This Dockerfile never switches away from the default root user. A container escape or a compromised process in this image runs as root on the host's container runtime.",
		fix:      "Add a USER instruction for a non-root user before the final CMD/ENTRYPOINT (create one with RUN adduser/useradd if the base image doesn't provide one).",
	}
	userRootRule = rule{
		id:       "docker.user_root",
		severity: core.Medium,
		title:    "Docker runs as root",
		message:  "This Dockerfile explicitly sets USER root (or an equivalent UID 0) for its final stage. A container escape or a compromised process in this image runs as root on the host's container runtime.",
		fix:      "Switch to a non-root user before the final CMD/ENTRYPOINT (create one with RUN adduser/useradd if the base image doesn't provide one).",
	}
)

func finding(r rule, path string, line int) core.Finding {
	return core.Finding{
		ID:       r.id,
		Severity: r.severity,
		Title:    r.title,
		Message:  r.message,
		Fix:      r.fix,
		File:     path,
		Line:     line,
		Category: "docker",
	}
}
