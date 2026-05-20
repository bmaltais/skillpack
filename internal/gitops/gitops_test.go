package gitops

import "testing"

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo.git", true},
		{"https://github.com/user/repo.git", false},
		{"http://github.com/user/repo.git", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsSSHURL(tt.url); got != tt.want {
			t.Errorf("IsSSHURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestPathUnderPrefix(t *testing.T) {
	tests := []struct {
		filePath string
		prefix   string
		want     bool
	}{
		{"coding/debugger/SKILL.md", "coding/debugger", true},
		{"coding/debugger", "coding/debugger", true},
		{"coding/debugger-v2/SKILL.md", "coding/debugger", false},
		{"other/file.go", "coding/debugger", false},
		{"", "coding/debugger", false},
		{"coding/debugger/file.go", "", false},
	}
	for _, tt := range tests {
		if got := pathUnderPrefix(tt.filePath, tt.prefix); got != tt.want {
			t.Errorf("pathUnderPrefix(%q, %q) = %v, want %v", tt.filePath, tt.prefix, got, tt.want)
		}
	}
}

func TestSafeShort(t *testing.T) {
	tests := []struct {
		sha  string
		want string
	}{
		{"abcdef1234567890", "abcdef12"},
		{"short", "short"},
		{"", ""},
		{"12345678", "12345678"},
	}
	for _, tt := range tests {
		if got := safeShort(tt.sha); got != tt.want {
			t.Errorf("safeShort(%q) = %q, want %q", tt.sha, got, tt.want)
		}
	}
}
