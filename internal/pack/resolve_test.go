package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/state"
)

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"/home/user/packs/go-dev", true},
		{"./packs/go-dev", true},
		{"../packs/go-dev", true},
		{"~/packs/go-dev", true},
		{"my-repo/packs/go-dev", false},
		{"https://example.com/pack.yaml", false},
	}
	for _, tc := range tests {
		if got := isLocalPath(tc.addr); got != tc.want {
			t.Errorf("isLocalPath(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestAddressFromName_NormalisesName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"My Pack", "my-pack"},
		{"already-lower", "already-lower"},
		{"UPPER CASE NAME", "upper-case-name"},
	}
	for _, tc := range tests {
		if got := addressFromName(tc.in); got != tc.want {
			t.Errorf("addressFromName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFetchURL_RejectsNonHTTPS(t *testing.T) {
	_, err := fetchURL("http://example.com/pack.yaml")
	if err == nil || !strings.Contains(err.Error(), "only HTTPS") {
		t.Errorf("fetchURL(http) error = %v, want HTTPS-only rejection", err)
	}
}

func TestResolve_FromFilepath(t *testing.T) {
	packDir := t.TempDir()
	content := `
name: test-pack
description: A test pack
repos:
  - name: my-repo
    url: https://github.com/example/my-repo.git
skills:
  - my-repo/coding/debugger
`
	if err := os.WriteFile(filepath.Join(packDir, "pack.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	def, err := Resolve(packDir, &state.State{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if def.Pack.Name != "test-pack" {
		t.Errorf("Pack.Name = %q, want test-pack", def.Pack.Name)
	}
	if def.Address != "test-pack" {
		t.Errorf("Address = %q, want test-pack", def.Address)
	}
	if len(def.Pack.Skills) != 1 || def.Pack.Skills[0] != "my-repo/coding/debugger" {
		t.Errorf("Skills = %v", def.Pack.Skills)
	}
}

func TestResolve_UnknownRegisteredAddress(t *testing.T) {
	if _, err := Resolve("no-such-repo/packs/x", &state.State{Repos: map[string]state.RepoRecord{}}); err == nil {
		t.Error("expected error for a pack in an unregistered repo")
	}
}
