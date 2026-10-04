package findmy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func aliasPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "findmy-cli", "aliases.json")
}

func LoadAliases() (map[string]string, error) {
	data, err := os.ReadFile(aliasPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read aliases: %w", err)
	}
	var aliases map[string]string
	if err := json.Unmarshal(data, &aliases); err != nil {
		return nil, fmt.Errorf("parse aliases: %w", err)
	}
	if aliases == nil {
		return nil, fmt.Errorf("aliases must be a JSON object")
	}
	return aliases, nil
}

func SaveAliases(m map[string]string) error {
	p := aliasPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// ResolveAlias returns the device name for a given alias (case-insensitive).
// If no alias matches, returns the input unchanged.
func ResolveAlias(input string) (string, error) {
	m, err := LoadAliases()
	if err != nil {
		return "", err
	}
	key := strings.ToLower(strings.TrimSpace(input))
	for k, v := range m {
		if strings.ToLower(k) == key {
			return v, nil
		}
	}
	return input, nil
}
