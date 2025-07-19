# Enhanced OAuth2 & API Key Scoping System Implementation Plan

## Overview

Design and implement a comprehensive multi-scope system for both OAuth2 applications and API keys that builds on Coder's existing RBAC infrastructure to provide fine-grained authorization control.

## Current State Analysis

### Existing Systems

- **API Keys**: Single `scope` enum field (`all` | `application_connect`) in `api_keys` table
- **OAuth2 Apps**: Single `scope` text field in `oauth2_provider_apps` table
- **RBAC System**: 33+ resource types with specific actions, policy-based authorization using OPA
- **Built-in Scopes**: `ScopeAll`, `ScopeApplicationConnect`, `ScopeNoUserData`

## Implementation Plan

### Phase 1: Database Schema Migration

#### 1.1 Extend Existing Scope Enum

```sql
-- Extend existing api_key_scope enum (instead of creating v2)
ALTER TYPE api_key_scope ADD VALUE 'user:read';
ALTER TYPE api_key_scope ADD VALUE 'user:write';
ALTER TYPE api_key_scope ADD VALUE 'workspace:read';
ALTER TYPE api_key_scope ADD VALUE 'workspace:write';
ALTER TYPE api_key_scope ADD VALUE 'workspace:ssh';
ALTER TYPE api_key_scope ADD VALUE 'workspace:apps';
ALTER TYPE api_key_scope ADD VALUE 'template:read';
ALTER TYPE api_key_scope ADD VALUE 'template:write';
ALTER TYPE api_key_scope ADD VALUE 'organization:read';
ALTER TYPE api_key_scope ADD VALUE 'organization:write';
ALTER TYPE api_key_scope ADD VALUE 'audit:read';
ALTER TYPE api_key_scope ADD VALUE 'system:read';
ALTER TYPE api_key_scope ADD VALUE 'system:write';
```

#### 1.2 API Keys Migration

```sql
-- Add new column with enum array (reusing existing enum)
ALTER TABLE api_keys ADD COLUMN scopes api_key_scope[];

-- Migrate existing data
UPDATE api_keys SET scopes = ARRAY[scope] WHERE scopes IS NULL;

-- Make non-null with default
ALTER TABLE api_keys ALTER COLUMN scopes SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN scopes SET DEFAULT '{"all"}';

-- Drop old single scope column
ALTER TABLE api_keys DROP COLUMN scope;
```

#### 1.3 OAuth2 Apps Migration

```sql
-- Add new column with enum array (reusing existing enum)
ALTER TABLE oauth2_provider_apps ADD COLUMN scopes api_key_scope[];

-- Migrate existing data (split space-delimited scopes and convert to enum)
UPDATE oauth2_provider_apps SET scopes =
    CASE
        WHEN scope = '' THEN '{}'::api_key_scope[]
        ELSE string_to_array(scope, ' ')::api_key_scope[]
    END
WHERE scopes IS NULL;

-- Make non-null with default
ALTER TABLE oauth2_provider_apps ALTER COLUMN scopes SET NOT NULL;
ALTER TABLE oauth2_provider_apps ALTER COLUMN scopes SET DEFAULT '{}';

-- Drop old column
ALTER TABLE oauth2_provider_apps DROP COLUMN scope;
```

### Phase 2: Core Scope Infrastructure

#### 2.1 Built-in Scope Definitions with Resource Prefixes

Using resource-based prefixes for clarity:

- `user:read` - Read user profile information
- `user:write` - Update user profile (self-service)
- `workspace:read` - Read workspaces and workspace metadata
- `workspace:write` - Create, update, delete workspaces
- `workspace:ssh` - SSH access to workspaces
- `workspace:apps` - Connect to workspace applications
- `template:read` - Read templates and template metadata
- `template:write` - Create, update, delete templates (admin-level)
- `organization:read` - Read organization information
- `organization:write` - Manage organization resources
- `audit:read` - Read audit logs (admin-level)
- `system:read` - Read system information
- `system:write` - Manage system resources (owner-level)
- `all` - Full access (backward compatibility)
- `application_connect` - Legacy scope for backward compatibility

#### 2.2 Enhanced Reusable Scope Building System

Enhanced helper function with support for site and org permissions:

```go
// Extend existing ScopeName constants
const (
    // Existing scopes (unchanged)
    ScopeAll                ScopeName = "all"
    ScopeApplicationConnect ScopeName = "application_connect"
    ScopeNoUserData         ScopeName = "no_user_data"

    // New granular scopes
    ScopeUserRead           ScopeName = "user:read"
    ScopeUserWrite          ScopeName = "user:write"
    ScopeWorkspaceRead      ScopeName = "workspace:read"
    ScopeWorkspaceWrite     ScopeName = "workspace:write"
    ScopeWorkspaceSSH       ScopeName = "workspace:ssh"
    ScopeWorkspaceApps      ScopeName = "workspace:apps"
    ScopeTemplateRead       ScopeName = "template:read"
    ScopeTemplateWrite      ScopeName = "template:write"
    ScopeOrganizationRead   ScopeName = "organization:read"
    ScopeOrganizationWrite  ScopeName = "organization:write"
    ScopeAuditRead          ScopeName = "audit:read"
    ScopeSystemRead         ScopeName = "system:read"
    ScopeSystemWrite        ScopeName = "system:write"
)

// Additional permissions for write scopes
type AdditionalPermissions struct {
    Site map[string][]policy.Action           // Site-level permissions
    Org  map[string][]Permission              // Organization-level permissions
    User []Permission                         // User-level permissions
}

// Enhanced reusable function to build write scopes from read scopes
func buildWriteScopeFromRead(readScopeName ScopeName, writeScopeName ScopeName, displayName string, additionalPerms AdditionalPermissions) Scope {
    // Deep copy the read scope
    readScope := builtinScopes[readScopeName]
    writeScope := Scope{
        Role: Role{
            Identifier:  RoleIdentifier{Name: fmt.Sprintf("Scope_%s", writeScopeName)},
            DisplayName: displayName,
            Site:        make(Permissions),
            Org:         make(map[string][]Permission),
            User:        make([]Permission, len(readScope.Role.User)),
        },
        AllowIDList: make([]string, len(readScope.AllowIDList)),
    }

    // Deep copy read permissions - Site level
    for resource, actions := range readScope.Role.Site {
        writeScope.Role.Site[resource] = make([]policy.Action, len(actions))
        copy(writeScope.Role.Site[resource], actions)
    }

    // Deep copy read permissions - Org level
    for resource, perms := range readScope.Role.Org {
        writeScope.Role.Org[resource] = make([]Permission, len(perms))
        copy(writeScope.Role.Org[resource], perms)
    }

    // Deep copy read permissions - User level
    copy(writeScope.Role.User, readScope.Role.User)

    // Deep copy AllowIDList
    copy(writeScope.AllowIDList, readScope.AllowIDList)

    // Add additional site permissions
    for resource, actions := range additionalPerms.Site {
        if existing, exists := writeScope.Role.Site[resource]; exists {
            // Merge with existing permissions (avoid duplicates)
            combined := append(existing, actions...)
            writeScope.Role.Site[resource] = removeDuplicateActions(combined)
        } else {
            writeScope.Role.Site[resource] = actions
        }
    }

    // Add additional org permissions
    for resource, perms := range additionalPerms.Org {
        if existing, exists := writeScope.Role.Org[resource]; exists {
            // Merge with existing permissions (avoid duplicates)
            combined := append(existing, perms...)
            writeScope.Role.Org[resource] = removeDuplicatePermissions(combined)
        } else {
            writeScope.Role.Org[resource] = perms
        }
    }

    // Add additional user permissions
    if len(additionalPerms.User) > 0 {
        writeScope.Role.User = append(writeScope.Role.User, additionalPerms.User...)
        writeScope.Role.User = removeDuplicatePermissions(writeScope.Role.User)
    }

    return writeScope
}

// Helper function to remove duplicate actions
func removeDuplicateActions(actions []policy.Action) []policy.Action {
    seen := make(map[policy.Action]bool)
    result := []policy.Action{}
    for _, action := range actions {
        if !seen[action] {
            seen[action] = true
            result = append(result, action)
        }
    }
    return result
}

// Helper function to remove duplicate permissions
func removeDuplicatePermissions(permissions []Permission) []Permission {
    seen := make(map[string]bool)
    result := []Permission{}
    for _, perm := range permissions {
        key := fmt.Sprintf("%s:%s", perm.ResourceType, perm.Action)
        if !seen[key] {
            seen[key] = true
            result = append(result, perm)
        }
    }
    return result
}

// Build all scopes with read/write pairs grouped together
var builtinScopes = map[ScopeName]Scope{
    // Existing scopes (unchanged)
    ScopeAll: { /* existing definition */ },
    ScopeApplicationConnect: { /* existing definition */ },
    ScopeNoUserData: { /* existing definition */ },

    // User scopes (read + write pair)
    ScopeUserRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_user:read"},
            DisplayName: "Read user profile",
            Site: Permissions(map[string][]policy.Action{
                ResourceUser.Type: {policy.ActionReadPersonal},
            }),
            Org:  map[string][]Permission{},
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeUserWrite: buildWriteScopeFromRead(
        ScopeUserRead,
        ScopeUserWrite,
        "Manage user profile",
        AdditionalPermissions{
            Site: map[string][]policy.Action{
                ResourceUser.Type: {policy.ActionUpdatePersonal},
            },
            Org:  map[string][]Permission{},
            User: []Permission{},
        },
    ),

    // Workspace scopes (read + write pair)
    ScopeWorkspaceRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_workspace:read"},
            DisplayName: "Read workspaces",
            Site: Permissions(map[string][]policy.Action{
                ResourceWorkspace.Type: {policy.ActionRead},
            }),
            Org: map[string][]Permission{
                ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionRead}},
            },
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeWorkspaceWrite: buildWriteScopeFromRead(
        ScopeWorkspaceRead,
        ScopeWorkspaceWrite,
        "Manage workspaces",
        AdditionalPermissions{
            Site: map[string][]policy.Action{
                ResourceWorkspace.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
            },
            Org: map[string][]Permission{
                ResourceWorkspace.Type: {
                    {ResourceType: ResourceWorkspace.Type, Action: policy.ActionCreate},
                    {ResourceType: ResourceWorkspace.Type, Action: policy.ActionUpdate},
                    {ResourceType: ResourceWorkspace.Type, Action: policy.ActionDelete},
                },
            },
            User: []Permission{},
        },
    ),

    // Workspace special scopes (SSH and Apps)
    ScopeWorkspaceSSH: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_workspace:ssh"},
            DisplayName: "SSH to workspaces",
            Site: Permissions(map[string][]policy.Action{
                ResourceWorkspace.Type: {policy.ActionSSH},
            }),
            Org: map[string][]Permission{
                ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionSSH}},
            },
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeWorkspaceApps: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_workspace:apps"},
            DisplayName: "Connect to workspace applications",
            Site: Permissions(map[string][]policy.Action{
                ResourceWorkspace.Type: {policy.ActionApplicationConnect},
            }),
            Org: map[string][]Permission{
                ResourceWorkspace.Type: {{ResourceType: ResourceWorkspace.Type, Action: policy.ActionApplicationConnect}},
            },
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    // Template scopes (read + write pair)
    ScopeTemplateRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_template:read"},
            DisplayName: "Read templates",
            Site: Permissions(map[string][]policy.Action{
                ResourceTemplate.Type: {policy.ActionRead},
            }),
            Org: map[string][]Permission{
                ResourceTemplate.Type: {{ResourceType: ResourceTemplate.Type, Action: policy.ActionRead}},
            },
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeTemplateWrite: buildWriteScopeFromRead(
        ScopeTemplateRead,
        ScopeTemplateWrite,
        "Manage templates",
        AdditionalPermissions{
            Site: map[string][]policy.Action{
                ResourceTemplate.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
            },
            Org: map[string][]Permission{
                ResourceTemplate.Type: {
                    {ResourceType: ResourceTemplate.Type, Action: policy.ActionCreate},
                    {ResourceType: ResourceTemplate.Type, Action: policy.ActionUpdate},
                    {ResourceType: ResourceTemplate.Type, Action: policy.ActionDelete},
                },
            },
            User: []Permission{},
        },
    ),

    // Organization scopes (read + write pair)
    ScopeOrganizationRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_organization:read"},
            DisplayName: "Read organization",
            Site: Permissions(map[string][]policy.Action{
                ResourceOrganization.Type: {policy.ActionRead},
            }),
            Org: map[string][]Permission{
                ResourceOrganization.Type: {{ResourceType: ResourceOrganization.Type, Action: policy.ActionRead}},
            },
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeOrganizationWrite: buildWriteScopeFromRead(
        ScopeOrganizationRead,
        ScopeOrganizationWrite,
        "Manage organization",
        AdditionalPermissions{
            Site: map[string][]policy.Action{
                ResourceOrganization.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
            },
            Org: map[string][]Permission{
                ResourceOrganization.Type: {
                    {ResourceType: ResourceOrganization.Type, Action: policy.ActionCreate},
                    {ResourceType: ResourceOrganization.Type, Action: policy.ActionUpdate},
                    {ResourceType: ResourceOrganization.Type, Action: policy.ActionDelete},
                },
            },
            User: []Permission{},
        },
    ),

    // Audit scopes (read only - no write needed)
    ScopeAuditRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_audit:read"},
            DisplayName: "Read audit logs",
            Site: Permissions(map[string][]policy.Action{
                ResourceAuditLog.Type: {policy.ActionRead},
            }),
            Org:  map[string][]Permission{},
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    // System scopes (read + write pair)
    ScopeSystemRead: {
        Role: Role{
            Identifier:  RoleIdentifier{Name: "Scope_system:read"},
            DisplayName: "Read system information",
            Site: Permissions(map[string][]policy.Action{
                ResourceSystem.Type: {policy.ActionRead},
            }),
            Org:  map[string][]Permission{},
            User: []Permission{},
        },
        AllowIDList: []string{policy.WildcardSymbol},
    },

    ScopeSystemWrite: buildWriteScopeFromRead(
        ScopeSystemRead,
        ScopeSystemWrite,
        "Manage system",
        AdditionalPermissions{
            Site: map[string][]policy.Action{
                ResourceSystem.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
            },
            Org:  map[string][]Permission{},
            User: []Permission{},
        },
    ),
}
```

### Phase 3: Authorization Integration

#### 3.1 Multi-Scope Validation

- **Scope combination**: Merge permissions from multiple scopes
- **Hierarchy support**: Higher scopes automatically include lower scope permissions
- **Validation**: Ensure scope combinations are valid and don't conflict

#### 3.2 API & Database Layer Updates

- **Update SDKs**: Change `APIKeyScope` to `[]APIKeyScope` in `codersdk`
- **Update database queries**: Modify OAuth2 and API key queries to handle enum arrays
- **Update audit tables**: Add scopes array support to audit logging

### Phase 4: Backward Compatibility & API Evolution

#### 4.1 Dual Field Support in API Responses

```go
type APIKey struct {
    ID              string        `json:"id"`
    UserID          uuid.UUID     `json:"user_id"`
    // ... other fields
    Scopes          []APIKeyScope `json:"scopes"`           // New array field
    Scope           APIKeyScope   `json:"scope"`            // Legacy field for compatibility
    // ... other fields
}

// When returning API response:
func (k *APIKey) populateCompatibilityField() {
    if len(k.Scopes) == 1 {
        k.Scope = k.Scopes[0]
    } else if len(k.Scopes) > 1 {
        // Join multiple scopes with space (OAuth2 standard)
        k.Scope = APIKeyScope(strings.Join(scopesToStrings(k.Scopes), " "))
    }
}
```

#### 4.2 API Input Handling

- **Accept both formats**: Support both `scope` (string) and `scopes` (array) in requests
- **Automatic conversion**: Convert single scope to array internally
- **Validation**: Ensure provided scopes are valid enum values

### Phase 5: Migration & Validation

#### 5.1 Schema Constraints

```sql
-- Add constraint to ensure scopes array is not empty for active tokens
ALTER TABLE api_keys ADD CONSTRAINT api_keys_scopes_not_empty
    CHECK (array_length(scopes, 1) > 0);

-- Add constraint to ensure valid scope combinations
ALTER TABLE api_keys ADD CONSTRAINT api_keys_scopes_valid
    CHECK (NOT ('all' = ANY(scopes) AND array_length(scopes, 1) > 1));
```

#### 5.2 Validation Logic

- **Enum validation**: PostgreSQL automatically validates enum values
- **Combination validation**: Prevent `all` scope from being combined with others
- **Hierarchy validation**: Ensure higher scopes include lower scope permissions

## Code Structure

```
coderd/rbac/
├── scopes.go                # Enhanced scope infrastructure with organized read/write pairs
└── scope_validator.go       # Multi-scope validation logic

codersdk/
├── apikey.go               # Updated with scopes array + backward compatibility
└── oauth2.go               # Updated with scopes array

coderd/database/
├── migrations/             # Schema migration files
└── queries/               # Updated SQL queries for enum arrays
```

## Success Criteria

1. **Type safety**: Enum arrays prevent invalid scope values
2. **Multi-scope support**: Both API keys and OAuth2 apps support multiple scopes
3. **Organized scope definitions**: Read/write pairs grouped together for clarity
4. **Comprehensive permission support**: Site, org, and user-level permissions in helper function
5. **Reusable scope building**: DRY principle applied to scope creation
6. **Backward compatibility**: Existing single-scope tokens continue working
7. **Permission embedding**: Higher scopes automatically include lower scope permissions
8. **Dual API support**: Both `scope` and `scopes` fields in responses
9. **Fine-grained control**: GitHub-style scope granularity for permissions
10. **Enum reuse**: Extends existing enum instead of creating new types

## Migration Strategy

1. **Phase 1**: Extend existing enum and migrate database schema
2. **Phase 2**: Update Go code with organized, enhanced reusable scope building system
3. **Phase 3**: Frontend updates to support multi-scope selection
4. **Phase 4**: Deprecate single-scope APIs (with warnings)
5. **Phase 5**: Remove single-scope support (future major version)

This plan provides a well-organized, comprehensive approach to scope building with read/write pairs grouped together, full support for site, org, and user-level permissions, while maintaining type safety and full backward compatibility.
