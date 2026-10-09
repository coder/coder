package dynamicparameters

import (
	"encoding/json"
	"strings"

	"github.com/coder/coder/v2/coderd/database"
)

// MissingModuleFiles reports whether coderd cannot give the renderer the module
// files this template version needs. See missingModuleFiles.
func (r *dynamicRenderer) MissingModuleFiles() bool {
	if r.data == nil {
		return false
	}

	return missingModuleFiles(r.data.terraformValues)
}

// The guard in ResolveParameters degrades silently if this is ever dropped, so
// assert the implementation at compile time.
var _ incompleteRenderReporter = (*dynamicRenderer)(nil)

// missingModuleFiles reports whether a template version depends on module files
// that coderd does not have.
//
// The render overlays `.terraform/modules` from
// template_version_terraform_values.cached_module_files. Only remotely sourced
// modules live there; modules with a local source are part of the template
// archive itself and always reach the renderer. A version whose modules are all
// local therefore stores no archive and still renders completely, so the cached
// plan is used to decide whether the version depends on a remote module at all.
//
// This is coderd's own evidence. It does not depend on the renderer inferring a
// failed module resolution from the blocks it can see.
func missingModuleFiles(values *database.TemplateVersionTerraformValue) bool {
	if values == nil || values.CachedModuleFiles.Valid {
		return false
	}

	return planUsesRemoteModules(values.CachedPlan)
}

// moduleCall is the subset of a `terraform show -json` module call this package
// needs. Calls nest, because a local module may itself call a remote one.
type moduleCall struct {
	Source string `json:"source"`
	Module struct {
		ModuleCalls map[string]moduleCall `json:"module_calls"`
	} `json:"module"`
}

// planUsesRemoteModules reports whether a plan declares at least one module
// whose source must be fetched, at any depth. It returns false when the plan is
// absent or cannot be read, because absence of evidence is not evidence that
// remote modules are used; the render diagnostics remain as a fallback.
func planUsesRemoteModules(cachedPlan []byte) bool {
	if len(cachedPlan) == 0 {
		return false
	}

	var plan struct {
		Configuration struct {
			RootModule struct {
				ModuleCalls map[string]moduleCall `json:"module_calls"`
			} `json:"root_module"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(cachedPlan, &plan); err != nil {
		return false
	}

	return anyRemoteModule(plan.Configuration.RootModule.ModuleCalls)
}

func anyRemoteModule(calls map[string]moduleCall) bool {
	for _, call := range calls {
		if isRemoteModuleSource(call.Source) {
			return true
		}
		if anyRemoteModule(call.Module.ModuleCalls) {
			return true
		}
	}

	return false
}

// isRemoteModuleSource reports whether terraform stores this module under
// .terraform/modules rather than reading it from the template archive. Local
// sources are the paths terraform documents as local: those beginning with ./
// or ../.
func isRemoteModuleSource(source string) bool {
	if source == "" {
		return false
	}

	return !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../")
}
