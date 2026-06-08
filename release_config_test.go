package nmanager

import (
	"os"
	"strings"
	"testing"
)

func TestGoReleaserSnapshotPackageVersion(t *testing.T) {
	config, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	text := string(config)
	if !strings.Contains(text, "\ngit:") || !strings.Contains(text, "\n  ignore_tags:\n    - snapshot\n") {
		t.Fatal("GoReleaser must ignore the snapshot tag so it is not used as the package version base")
	}

	const template = "{{ incpatch .Version }}-snapshot.{{ .ShortCommit }}"
	if !strings.Contains(text, "\nsnapshot:") || !strings.Contains(text, "\n  version_template: '"+template+"'\n") {
		t.Fatalf(".goreleaser.yml must configure snapshot.version_template as %q", template)
	}

	version := strings.NewReplacer(
		"{{ incpatch .Version }}", "0.0.1",
		"{{ .ShortCommit }}", "abc1234",
	).Replace(template)
	if version == "" || version[0] < '0' || version[0] > '9' {
		t.Fatalf("snapshot package version must start with a digit for Debian packages, got %q", version)
	}
}
