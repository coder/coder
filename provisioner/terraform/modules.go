package terraform
import (
	"fmt"
	"errors"
	"encoding/json"
	"os"
	"path/filepath"
	"github.com/coder/coder/v2/provisionersdk/proto"
)
type module struct {
	Source  string `json:"Source"`
	Version string `json:"Version"`
	Key     string `json:"Key"`
}
type modulesFile struct {
	Modules []*module `json:"Modules"`
}
func getModulesFilePath(workdir string) string {
	return filepath.Join(workdir, ".terraform", "modules", "modules.json")
}
func parseModulesFile(filePath string) ([]*proto.Module, error) {
	modules := &modulesFile{}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read modules file: %w", err)
	}
	if err := json.Unmarshal(data, modules); err != nil {
		return nil, fmt.Errorf("unmarshal modules file: %w", err)
	}
	protoModules := make([]*proto.Module, len(modules.Modules))
	for i, m := range modules.Modules {
		protoModules[i] = &proto.Module{Source: m.Source, Version: m.Version, Key: m.Key}
	}
	return protoModules, nil
}
// getModules returns the modules from the modules file if it exists.
// It returns nil if the file does not exist.
// Modules become available after terraform init.
func getModules(workdir string) ([]*proto.Module, error) {
	filePath := getModulesFilePath(workdir)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}
	modules, err := parseModulesFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parse modules file: %w", err)
	}
	filteredModules := []*proto.Module{}
	for _, m := range modules {
		// Empty string means root module. It's always present, so we skip it.
		if m.Source == "" {
			continue
		}
		filteredModules = append(filteredModules, m)
	}
	return filteredModules, nil
}
