package docker

import (
	"testing"

	"github.com/xchebila/reposcan/core"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		wantIDs []string
	}{
		{
			name:    "not a dockerfile is ignored entirely",
			path:    "main.go",
			content: "FROM ubuntu\n",
			wantIDs: nil,
		},
		{
			name:    "no tag defaults to latest",
			path:    "Dockerfile",
			content: "FROM ubuntu\nUSER app\n",
			wantIDs: []string{"docker.latest_tag"},
		},
		{
			name:    "explicit latest tag",
			path:    "Dockerfile",
			content: "FROM ubuntu:latest\nUSER app\n",
			wantIDs: []string{"docker.latest_tag"},
		},
		{
			name:    "pinned tag is clean",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nUSER app\n",
			wantIDs: nil,
		},
		{
			name:    "digest pin is clean even with no explicit tag",
			path:    "Dockerfile",
			content: "FROM ubuntu@sha256:abcd1234\nUSER app\n",
			wantIDs: nil,
		},
		{
			name: "multi-stage FROM builder is not a floating-tag finding",
			path: "Dockerfile",
			content: "FROM golang:1.25 AS builder\n" +
				"RUN go build -o app .\n" +
				"FROM builder\n" +
				"USER app\n",
			// "FROM builder" would look like an untagged pull if stage names
			// weren't excluded -- only golang:1.25 (pinned) is a candidate,
			// and it's pinned, so no finding at all.
			wantIDs: nil,
		},
		{
			name:    "scratch is exempt from the tag check",
			path:    "Dockerfile",
			content: "FROM scratch\nUSER app\n",
			wantIDs: nil,
		},
		{
			name:    "ADD of a local file instead of COPY",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nADD app.py /app/app.py\nUSER app\n",
			wantIDs: []string{"docker.add_instead_of_copy"},
		},
		{
			name:    "ADD fetching a URL is legitimate ADD usage",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nADD https://example.com/file.tar.gz /app/\nUSER app\n",
			wantIDs: nil,
		},
		{
			name:    "ADD auto-extracting a local archive is legitimate ADD usage",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nADD app.tar.gz /app/\nUSER app\n",
			wantIDs: nil,
		},
		{
			name:    "no USER instruction at all",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nRUN echo hi\n",
			wantIDs: []string{"docker.no_nonroot_user"},
		},
		{
			name:    "distroless nonroot tag is exempt from the no-USER check",
			path:    "Dockerfile",
			content: "FROM gcr.io/distroless/static-debian12:nonroot-amd64\n",
			wantIDs: nil,
		},
		{
			name:    "explicit USER root",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nUSER root\n",
			wantIDs: []string{"docker.user_root"},
		},
		{
			name:    "explicit USER 0 is root by UID",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nUSER 0\n",
			wantIDs: []string{"docker.user_root"},
		},
		{
			name:    "explicit non-root USER is clean",
			path:    "Dockerfile",
			content: "FROM ubuntu:22.04\nUSER app\n",
			wantIDs: nil,
		},
		{
			name: "only the final stage's USER matters",
			path: "Dockerfile",
			content: "FROM ubuntu:22.04 AS builder\n" +
				"USER root\n" +
				"RUN make build\n" +
				"FROM ubuntu:22.04\n" +
				"USER app\n",
			wantIDs: nil,
		},
	}

	a := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := a.Run(core.FileContext{Path: tt.path, Content: []byte(tt.content)})
			gotIDs := make([]string, len(findings))
			for i, f := range findings {
				gotIDs[i] = f.ID
				if f.Category != "docker" {
					t.Errorf("finding %s: Category = %q, want \"docker\"", f.ID, f.Category)
				}
			}
			assertSameIDs(t, gotIDs, tt.wantIDs)
		})
	}
}

func assertSameIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("finding IDs = %v, want %v", got, want)
	}
	wantSet := map[string]int{}
	for _, id := range want {
		wantSet[id]++
	}
	for _, id := range got {
		wantSet[id]--
	}
	for id, n := range wantSet {
		if n != 0 {
			t.Errorf("finding IDs = %v, want %v (mismatch on %q)", got, want, id)
			return
		}
	}
}
