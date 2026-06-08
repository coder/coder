# Sensitive Parameters: Provider Implementation Notes

> [!NOTE]
> This is an internal/implementation reference for the prototype on branch
> `scott/prototype-sensitive-params`. It documents the upstream changes required
> for sensitive parameters to be fully usable end to end. It is not user-facing
> guidance.

The Coder-side plumbing for sensitive parameters is implemented, but a template
author cannot yet declare `sensitive = true` on a `coder_parameter` until the
following external components add support.

## 1. terraform-provider-coder

File: `provider/parameter.go` (`coder/terraform-provider-coder`)

- Add a `sensitive` schema attribute to the `coder_parameter` data source
  (`schema.Schema{Type: schema.TypeBool, Optional: true, Default: false}`).
- Add a `Sensitive bool` field to the `Parameter` struct with the matching
  `mapstructure:"sensitive"` mapping (mirror how `Ephemeral` is handled,
  including the decode block in `ParameterFromSchema`/`fixDefaults`).
- Optionally enforce sane combinations, for example warn when `sensitive = true`
  is combined with non-ephemeral mutable parameters, since the value will be
  encrypted at rest but still persisted.

Once released, set the value in coder/coder at
`provisioner/terraform/resources.go` where the TODO is left:

```go
protoParam := &proto.RichParameter{
    // ...
    Ephemeral: param.Ephemeral,
    Sensitive: param.Sensitive, // enable once the provider exposes it
}
```

## 2. coder/preview (dynamic parameters)

File: `types/parameter.go` (`coder/preview`)

- Add `Sensitive bool` to the parameter type and populate it from the HCL
  `sensitive` attribute during extraction (`extract/parameter.go`), mirroring
  `Ephemeral`.

Once released, set the value in coder/coder at
`coderd/database/db2sdk/db2sdk.go` in the `PreviewParameter` converter:

```go
Order:     param.Order,
Ephemeral: param.Ephemeral,
Sensitive: param.Sensitive, // enable once preview exposes it
```

And source the persisted flag for the dynamic build flow in
`coderd/provisionerdserver/provisionerdserver.go` (already wired for the classic
flow via `richParameter.Sensitive`).

## 3. Coder-side surface already implemented

- DB: `template_version_parameters.sensitive`,
  `workspace_build_parameters.sensitive`, and
  `workspace_build_parameters.value_key_id` with a foreign key to
  `dbcrypt_keys(active_key_digest)` (migration `000517`), matching the user
  secrets encryption practice.
- Proto: `RichParameter.sensitive` (field 19).
- Persistence: classic import path persists the flag; sensitive build values are
  encrypted via the `dbcrypt` store wrapper and decrypted on read for the
  provisioner.
- API: `GET /workspacebuilds/{id}/parameters` redacts sensitive values; the
  user-prefill query excludes sensitive parameters.
- SDK/TS types updated.

## 4. Production follow-ups (not in prototype)

- Frontend: mask sensitive parameter inputs and indicate non-persistence in the
  create/update workspace forms (Storybook coverage).
- Audit/telemetry review to ensure sensitive values are not captured elsewhere
  (for example provisioner job logs).
