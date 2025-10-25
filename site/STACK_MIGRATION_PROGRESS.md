# Stack Component Migration Progress

This document tracks the progress of migrating from the Emotion Stack component to Tailwind CSS flex utilities across the frontend codebase.

## Overview

- **Total Files**: 110 files using Stack component
- **Total PRs Planned**: ~19 PRs
- **Strategy**: Migrate in logical, testable groups under 1000 lines per PR

## Migration Pattern

### Before (Emotion Stack):
```tsx
import { Stack } from "components/Stack/Stack";

<Stack direction="row" spacing={2} alignItems="center">
  <Child1 />
  <Child2 />
</Stack>
```

### After (Tailwind):
```tsx
// For row layout with spacing
<div className="flex flex-row items-center gap-4">
  <Child1 />
  <Child2 />
</div>

// For column layout with spacing
<div className="flex flex-col gap-4">
  <Child1 />
  <Child2 />
</div>
```

### Conversion Reference:
- `direction="row"` → `flex flex-row` or `flex-row`
- `direction="column"` → `flex flex-col`
- `spacing={0.5}` → `gap-1` (4px)
- `spacing={1}` → `gap-2` (8px)
- `spacing={2}` → `gap-4` (16px)
- `spacing={3}` → `gap-6` (24px)
- `spacing={4}` → `gap-8` (32px)
- `spacing={6}` → `gap-12` (48px)
- `alignItems="center"` → `items-center`
- `alignItems="flex-start"` → `items-start`
- `alignItems="baseline"` → `items-baseline`
- `justifyContent="center"` → `justify-center`
- `justifyContent="space-between"` → `justify-between`
- `wrap="wrap"` → `flex-wrap`

## Completed PRs

### ✅ PR #1: Resources Module - Agent Components (Completed)
**Commit**: `f7b6769f7`
**Files**: 4 files, 36 lines changed (15 insertions, 21 deletions)
**Status**: Committed to main branch

**Files migrated:**
1. ✅ `src/modules/resources/AgentLatency.tsx` (85 lines)
2. ✅ `src/modules/resources/AgentMetadata.tsx` (237 lines)
3. ✅ `src/modules/resources/AgentOutdatedTooltip.tsx` (85 lines)
4. ✅ `src/modules/resources/SubAgentOutdatedTooltip.tsx` (67 lines)

**Testing**:
- TypeScript checks: ✅ Passed
- Biome linting: ✅ Passed
- Storybook: ✅ AgentMetadata story still functional

**Key Changes**:
- Removed Stack imports from all 4 files
- Replaced Stack with div + Tailwind classes
- Converted spacing={1} → gap-2, spacing={0.5} → gap-1
- Converted direction="row" → flex-row, direction="column" → flex-col
- Maintained all existing functionality and props

## Planned PRs

### 🔄 PR #2: Resources Module - Agent Rows and Previews (~900 lines)
**Status**: Not started
**Files to migrate:**
1. src/modules/resources/AgentRowPreview.tsx (215 lines)
2. src/modules/resources/AgentRow.tsx (542 lines)
3. src/modules/resources/AgentDevcontainerCard.tsx (387 lines)

### 📋 PR #3: Resources Module - Resource Cards and Links (~450 lines)
**Status**: Not started
**Files to migrate:**
1. src/modules/resources/ResourceCard.tsx (187 lines)
2. src/modules/resources/Resources.tsx (52 lines)
3. src/modules/resources/AppLink/AppPreview.tsx
4. src/modules/resources/PortForwardButton.tsx (MUI Stack)
5. src/modules/resources/SSHButton/SSHButton.tsx

### 📋 PR #4: Common Components - Smaller Utilities (~400 lines)
**Status**: Not started
**Files to migrate:**
1. src/components/Badges/Badges.tsx
2. src/components/HelpTooltip/HelpTooltip.tsx
3. src/components/StackLabel/StackLabel.tsx
4. src/modules/dashboard/DeploymentBanner/DeploymentBannerView.tsx
5. src/modules/dashboard/Navbar/UserDropdown/UserDropdownContent.tsx

### 📋 PR #5: Common Components - Forms and Dialogs (~600 lines)
**Status**: Not started
**Files to migrate:**
1. src/components/Dialogs/DeleteDialog/DeleteDialog.tsx
2. src/components/FileUpload/FileUpload.tsx
3. src/components/Form/Form.tsx
4. src/components/RichParameterInput/RichParameterInput.tsx

### 📋 PR #6-19: Additional PRs (To be detailed)
- PR #6: Common Components - Page elements
- PR #7: Workspace Pages - Dialogs and Parameters
- PR #8: Workspace Pages - Main Views
- PR #9-10: Template Pages (split into 2 batches)
- PR #11-12: User Settings Pages (split into 2 batches)
- PR #13-14: Deployment Settings (split into 2 batches)
- PR #15: Organization Settings
- PR #16: Audit & Logging Pages
- PR #17: Dashboard & Auth Pages
- PR #18: Misc Pages
- PR #19: Final cleanup and Stack component removal

## Progress Statistics

- **Files Migrated**: 4 / 110 (3.6%)
- **PRs Completed**: 1 / 19 (5.3%)
- **Lines Changed**: 36 lines (15+, 21-)
- **Stack Imports Removed**: 4

## Notes

- Each PR is kept under 1000 lines to facilitate review
- Related components are grouped together for easier testing
- TypeScript checks and linting pass after each migration
- All existing functionality is preserved
- Storybook stories remain functional after migration

## Next Steps

1. Review and merge PR #1
2. Begin PR #2: Migrate AgentRowPreview, AgentRow, and AgentDevcontainerCard
3. Continue with subsequent PRs in order
4. After all components migrated, remove Stack component definition
