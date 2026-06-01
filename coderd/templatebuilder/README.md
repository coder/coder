# templatebuilder

Package `templatebuilder` implements the bundled module catalog for the guided
template creation workflow. It embeds module metadata (`module.json` manifests)
into the Coder binary via `go:embed` and provides functions to load and convert
them for the API layer.

## Directory layout

```
coderd/templatebuilder/
    catalog.go              # embed.FS, manifest types, LoadModules(), ToSDK()
    catalog_test.go
    modules/
        <module-id>/
            module.json     # module manifest (on-disk schema)
```

Each subdirectory under `modules/` represents a single catalog entry. The
directory must contain a `module.json` file. Directories without one are
silently skipped.

## module.json schema

```jsonc
{
  "id": "code-server",                    // unique module identifier
  "display_name": "code-server",          // human-readable name
  "description": "VS Code in the browser",
  "icon": "...",                           // path or URL to icon asset
  "category": "IDE",                       // grouping in the UI
  "tags": ["ide", "web"],                  // internal tags (not exposed in API)
  "compatible_os": ["linux"],              // OS filter against base template
  "conflicts_with": [],                    // module IDs that conflict
  "pinned_version": "1.2.3",              // exact version bundled with this release
  "variables": [
    {
      "name": "agent_id",
      "type": "string",                    // "string" | "number" | "bool"
      "description": "The Coder agent ID.",
      "required": true,
      "sensitive": false,
      "builder_managed": true              // wired automatically, hidden from users
    },
    {
      "name": "port",
      "type": "number",
      "description": "Port to run code-server on.",
      "default": "13337",
      "required": false,
      "sensitive": false,
      "builder_managed": false
    }
  ]
}
```

Key fields:

- **`builder_managed`**: Variables the compose engine injects automatically
  (e.g. `agent_id`). These are never shown to users.
- **`sensitive`**: Variables containing secrets. The builder does not collect
  these; they become bare Terraform `variable` blocks so values are supplied
  at workspace creation time.
- **`pinned_version`**: The exact module version shipped with this Coder
  release. The compose endpoint always emits this version; `latest` is never
  used.
- **`compatible_os`**: Matched against the base template's OS to filter
  incompatible modules.
- **`conflicts_with`**: Module IDs that should not be selected together. The
  UI surfaces a warning but does not block selection.

## Two type layers

| Type | Location | Purpose |
|---|---|---|
| `ModuleManifest` / `ModuleVariable` | `catalog.go` | On-disk `module.json` schema. Has `pinned_version`, `tags`. |
| `codersdk.TemplateBuilderModule` / `codersdk.TemplateBuilderModuleVariable` | `codersdk/templatebuilder.go` | API response type. Has `version` (mapped from `pinned_version`). No `tags`. |

`ModuleManifest.ToSDK()` handles the conversion.

## Adding a module

1. Create `modules/<module-id>/module.json` following the schema above.
2. Run `go build ./coderd/templatebuilder/` to verify the embed compiles.
3. Run `go test ./coderd/templatebuilder/` to verify parsing.

The catalog is bundled at build time. New modules or version bumps require a
Coder release to appear in the builder.
