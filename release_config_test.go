package nmanager

import (
	"os"
	"strings"
	"testing"
)

func TestGoReleaserRPMIncludesSELinuxPolicy(t *testing.T) {
	config, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	text := string(config)
	nfpmsStart := strings.Index(text, "\nnfpms:\n")
	if nfpmsStart < 0 {
		t.Fatal("GoReleaser config must define nFPM packages")
	}
	nfpmsBlock := text[nfpmsStart:]
	if end := strings.Index(nfpmsBlock, "\nchecksum:\n"); end >= 0 {
		nfpmsBlock = nfpmsBlock[:end]
	}

	policyBlockStart := strings.Index(nfpmsBlock, "      - src: packaging/selinux/nm_netbird.pp\n")
	if policyBlockStart < 0 {
		t.Fatal("RPM package must include the generated SELinux policy module")
	}
	policyBlock := nfpmsBlock[policyBlockStart:]
	if next := strings.Index(policyBlock[len("      - src: packaging/selinux/nm_netbird.pp\n"):], "\n      - src:"); next >= 0 {
		policyBlock = policyBlock[:len("      - src: packaging/selinux/nm_netbird.pp\n")+next]
	}

	for _, want := range []string{
		"dst: /usr/share/selinux/packages/nm_netbird.pp",
		"packager: rpm",
	} {
		if !strings.Contains(policyBlock, want) {
			t.Fatalf("SELinux policy package entry must contain %q", want)
		}
	}

	for _, useless := range []string{
		"src: packaging/selinux/Makefile",
		"src: packaging/selinux/README.md",
		"src: packaging/selinux/nm_netbird.fc",
		"src: packaging/selinux/nm_netbird.te",
	} {
		if strings.Contains(nfpmsBlock, useless) {
			t.Fatalf("SELinux source-only file should not be packaged: %s", useless)
		}
	}
}

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
