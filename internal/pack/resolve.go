package pack

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// Definition is a resolved Pack Recipe together with its canonical Pack Address.
type Definition struct {
	Pack    *Pack
	Address string
}

// Resolve turns addr into a Definition. addr may be:
//   - a registered pack address (e.g. "my-repo/packs/go-dev")
//   - an HTTPS URL to a raw pack.yaml file
//   - a local filepath to a pack.yaml file or a directory containing one
//
// URL and filepath packs get a synthetic address derived from the pack name.
func Resolve(addr string, st *state.State) (*Definition, error) {
	switch {
	case strings.HasPrefix(addr, "https://"):
		data, err := fetchURL(addr)
		if err != nil {
			return nil, fmt.Errorf("fetching pack.yaml from %s: %w", addr, err)
		}
		pk, err := Parse(data)
		if err != nil {
			return nil, err
		}
		return &Definition{Pack: pk, Address: addressFromName(pk.Name)}, nil

	case isLocalPath(addr):
		expanded, err := config.ExpandPath(addr)
		if err != nil {
			return nil, fmt.Errorf("expanding path %q: %w", addr, err)
		}
		info, statErr := os.Stat(expanded)
		if statErr != nil {
			return nil, fmt.Errorf("accessing %q: %w", expanded, statErr)
		}
		packFile := expanded
		if info.IsDir() {
			packFile = filepath.Join(expanded, "pack.yaml")
		}
		pk, err := ParseFile(packFile)
		if err != nil {
			return nil, err
		}
		return &Definition{Pack: pk, Address: addressFromName(pk.Name)}, nil

	default:
		packInfo, err := repo.FindPack(addr, st)
		if err != nil {
			return nil, err
		}
		pk, err := ParseFile(filepath.Join(packInfo.FullPath, "pack.yaml"))
		if err != nil {
			return nil, err
		}
		return &Definition{Pack: pk, Address: addr}, nil
	}
}

// isLocalPath returns true when addr looks like a filesystem path (cross-platform).
func isLocalPath(addr string) bool {
	return filepath.IsAbs(addr) ||
		strings.HasPrefix(addr, "./") ||
		strings.HasPrefix(addr, "../") ||
		strings.HasPrefix(addr, "~/")
}

// addressFromName builds a synthetic pack address from a pack name for URL/filepath packs.
func addressFromName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// fetchURL downloads the content at rawURL and returns the bytes.
// Only HTTPS URLs are accepted to prevent MITM tampering of downloaded pack.yaml.
func fetchURL(rawURL string) ([]byte, error) {
	if !strings.HasPrefix(rawURL, "https://") {
		return nil, fmt.Errorf("only HTTPS URLs are supported for pack.yaml downloads (got %q)", rawURL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rawURL) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}
