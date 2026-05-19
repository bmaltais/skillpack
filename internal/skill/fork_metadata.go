package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const forkMetadataFilename = ".skillpack-fork"

type forkMetadata struct {
	UpstreamAddr string `json:"upstream_addr"`
	UpstreamSHA  string `json:"upstream_sha"`
}

func writeForkMetadata(skillDir, upstreamAddr, upstreamSHA string) error {
	data, err := json.MarshalIndent(forkMetadata{
		UpstreamAddr: upstreamAddr,
		UpstreamSHA:  upstreamSHA,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding fork metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(skillDir, forkMetadataFilename), data, 0600); err != nil {
		return fmt.Errorf("writing fork metadata: %w", err)
	}
	return nil
}

func readForkMetadata(skillDir string) (*forkMetadata, error) {
	path := filepath.Join(skillDir, forkMetadataFilename)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", forkMetadataFilename, err)
	}

	var meta forkMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", forkMetadataFilename, err)
	}
	if meta.UpstreamAddr == "" || meta.UpstreamSHA == "" {
		return nil, fmt.Errorf("invalid %s: upstream_addr and upstream_sha are required", forkMetadataFilename)
	}
	return &meta, nil
}
