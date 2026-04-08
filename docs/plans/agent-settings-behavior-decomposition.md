# Plan: Decompose AgentSettingsBehaviorPageView + Remove Kyleosophy

**Research:** [agent-settings-behavior-decomposition-research.md](./agent-settings-behavior-decomposition-research.md)

## Problem

`AgentSettingsBehaviorPageView` is a 717-line god component with 30+ props, 10+ local state variables,
and 7 visual settings sections (6 unextracted). Each section has its own form state, derived state, and
event handlers, but they're all tangled in one render function. Adding new settings (like the Retention
Period added in PR #23833) increases complexity non-linearly — the shared render, growing state surface,
and expanding props interface mean each addition costs more than the last. Danielle flagged this in her
[review](https://github.com/coder/coder/pull/23833#pullrequestreview-4073859981).

Additionally, the Kyleosophy feature (alternative completion chimes added in PR #23891) is being removed.
The original completion chime (`chime.mp3`, `ChimeButton`, `playChime`/`maybePlayChime`) is kept.

## Decisions

All decisions recorded in [research document](./agent-settings-behavior-decomposition-research.md#decisions).

1. **Approach A** — extract all sections as independent components. (Research: Approach A)
2. **Preserve `isPromptSaving` coupling** — pass `isAnyPromptSaving: boolean` to both prompt
   components. Future endpoint consolidation may make this moot. (Research: Cross-Section Coupling)
3. **Duplicate `MutationCallbacks`** per component. Revisit shared extraction later if it looks noisy.
4. **Co-locate textarea constants** with the two prompt components that use them.
5. **Keep existing stories unchanged** — this is a pure refactor; existing stories are the regression net.
6. **Remove Kyleosophy only** — revert the additions from PR #23891. Keep the original chime feature
   (`chime.mp3`, `ChimeButton`, cross-tab dedup, `getChimeEnabled`/`setChimeEnabled`) intact.

### Components to extract (5 new files in `components/`)

- `PersonalInstructionsSettings` — user prompt textarea form
- `SystemInstructionsSettings` — admin system prompt textarea form + TextPreviewDialog
- `VirtualDesktopSettings` — admin toggle
- `WorkspaceAutostopSettings` — admin toggle + duration form
- `RetentionPeriodSettings` — admin toggle + number input form

## Implementation

This is a pure refactor + feature removal. Existing stories are the regression net.
Verify stories pass → extract one component → verify stories still pass → repeat.

### Baseline: Verify existing stories pass

- Run `cd site && pnpm tsc --noEmit` to confirm the starting state is green.

### Phase 0: Remove Kyleosophy (keep original chime)

Do this first since it reduces the god component's scope before we start extracting.

**Delete static audio assets (kyleosophy clips only):**

- `site/static/chime_1.mp3` through `site/static/chime_8.mp3` (8 files)
- **Keep** `site/static/chime.mp3` (the original completion chime)

**Revert kyleosophy additions in `utils/chime.ts`:**

- Remove `KYLEOSOPHY_PREFERENCE_KEY` constant
- Remove `getKylesophyEnabled()`, `setKylesophyEnabled()`, `isKylesophyForced()` functions
- Remove `KYLEOSOPHY_SOUNDS` array export
- Remove `lastSoundUrl` variable
- Revert `playChimeAudio(soundUrl = "/chime.mp3")` back to `playChimeAudio()` with
  hardcoded `/chime.mp3` (original signature before PR #23891)
- Revert `playChime(chatID, soundUrl?)` back to `playChime(chatID)` (no sound URL param)
- Revert `maybePlayChime` to call `playChime(chatID)` without kyleosophy branching
- Remove `chimeAudio?.pause()` before replacement (added for kyleosophy overlap prevention,
  not needed when sound URL never changes)
- Keep all original chime logic: `getChimeEnabled`, `setChimeEnabled`, `playChimeAudio`,
  `playChime`, `maybePlayChime`, `LOCK_HOLD_MS`, `_resetForTesting`, cross-tab dedup

**Revert kyleosophy additions in `utils/chime.test.ts`:**

- Remove `describe("getKylesophyEnabled / setKylesophyEnabled", ...)` test block
- Remove "uses a kyleosophy sound when kyleosophy is enabled" test
- Remove "uses default chime.mp3 when kyleosophy is disabled" test
- Remove kyleosophy imports (`getKylesophyEnabled`, `KYLEOSOPHY_SOUNDS`, `setKylesophyEnabled`)
- Keep all original chime tests

**Remove kyleosophy from `AgentSettingsBehaviorPageView.tsx`:**

- Remove kyleosophy import (lines 16-20: `getKylesophyEnabled`, `isKylesophyForced`,
  `setKylesophyEnabled`)
- Remove `kylesophyForced` and `kylesophyEnabled` state (lines 154-155)
- Remove the entire Kyleosophy `<div>` section + preceding `<hr>` in the JSX (lines 681-706)

**Remove kyleosophy assertion from `AgentSettingsBehaviorPageView.stories.tsx`:**

- Remove any story assertions referencing "completion chime" or "Kyleosophy" (line 471)

**Keep untouched:**

- `components/ChimeButton.tsx` — original chime toggle, not kyleosophy
- `components/ChimeButton.stories.tsx` — original chime stories
- `AgentsPage.tsx` — `maybePlayChime` call stays (original chime behavior)
- `AgentCreatePage.tsx` — `<ChimeButton />` stays (original chime toggle)

**Verify:** `cd site && pnpm tsc --noEmit` — no type errors.

### Phase 1: Simple toggles (no form state)

**1a. Extract `VirtualDesktopSettings`**

Create `site/src/pages/AgentsPage/components/VirtualDesktopSettings.tsx`:

- Move the Virtual Desktop `<div>` block into this component.
- Props: `desktopEnabledData`, `onSaveDesktopEnabled`, `isSavingDesktopEnabled`,
  `isSaveDesktopEnabledError`. No local state needed.
- Duplicate `MutationCallbacks` interface in this file.
- In `AgentSettingsBehaviorPageView`, replace the inline block with `<VirtualDesktopSettings ... />`.
- Remove the corresponding props from the parent's interface.
- Verify: `pnpm tsc --noEmit`, confirm `DesktopSetting` and `TogglesDesktop` stories still render.

### Phase 2: Admin form sections

**2a. Extract `RetentionPeriodSettings`**

Create `site/src/pages/AgentsPage/components/RetentionPeriodSettings.tsx`:

- Move the retention `<form>` block into this component.
- Move all retention local state (`localRetentionDays`, `retentionToggled`) and derived state
  (`serverRetentionDays`, `retentionDays`, `isRetentionEnabled`, `isRetentionDaysDirty`,
  `isRetentionDaysNegative`, `retentionDaysMaximum`, `isRetentionDaysOverMax`, `isRetentionDaysZero`)
  and handlers (`resetRetentionState`, `handleToggleRetention`, `handleRetentionDaysChange`,
  `handleSaveRetentionDays`) into this component.
- Duplicate `MutationCallbacks` interface in this file.
- Props: `retentionDaysData`, `isRetentionDaysLoading`, `isRetentionDaysLoadError`,
  `onSaveRetentionDays`, `isSavingRetentionDays`, `isSaveRetentionDaysError`.
- Remove these props from parent's interface.
- Verify: `pnpm tsc --noEmit`.

**2b. Extract `WorkspaceAutostopSettings`**

Create `site/src/pages/AgentsPage/components/WorkspaceAutostopSettings.tsx`:

- Move the autostop `<form>` block into this component.
- Move all autostop local state (`localTTLMs`, `autostopToggled`) and derived state
  (`serverTTLMs`, `ttlMs`, `isAutostopEnabled`, `isTTLDirty`, `maxTTLMs`, `isTTLOverMax`, `isTTLZero`)
  and handlers (`resetAutostopState`, `handleToggleAutostop`, `handleSaveChatWorkspaceTTL`,
  `handleTTLChange`) into this component.
- Duplicate `MutationCallbacks` interface in this file.
- Props: `workspaceTTLData`, `isWorkspaceTTLLoading`, `isWorkspaceTTLLoadError`,
  `onSaveWorkspaceTTL`, `isSavingWorkspaceTTL`, `isSaveWorkspaceTTLError`.
- Remove these props from parent's interface.
- Verify: `pnpm tsc --noEmit`, confirm `DefaultAutostop*` stories still render.

### Phase 3: Prompt sections (coupled)

**3a. Extract `PersonalInstructionsSettings`**

Create `site/src/pages/AgentsPage/components/PersonalInstructionsSettings.tsx`:

- Move the Personal Instructions `<form>` block into this component.
- Move local state: `localUserEdit`, `isUserPromptOverflowing`.
- Move derived state: `serverUserPrompt`, `userPromptDraft`, `userInvisibleCharCount`, `isUserPromptDirty`.
- Move handler: `handleSaveUserPrompt`.
- Co-locate textarea constants (`textareaMaxHeight`, `textareaBaseClassName`,
  `textareaOverflowClassName`) in this file.
- Duplicate `MutationCallbacks` interface in this file.
- Props: `userPromptData`, `onSaveUserPrompt`, `isSavingUserPrompt`, `isSaveUserPromptError`,
  `isAnyPromptSaving: boolean`.
- The component uses `isAnyPromptSaving` (not just its own `isSavingUserPrompt`) to disable the
  textarea and buttons, preserving the existing coupling behavior.
- Remove user prompt props from parent's interface.
- Verify: `pnpm tsc --noEmit`.

**3b. Extract `SystemInstructionsSettings`**

Create `site/src/pages/AgentsPage/components/SystemInstructionsSettings.tsx`:

- Move the System Instructions `<form>` block and the `TextPreviewDialog` into this component.
- Move local state: `localEdit`, `localIncludeDefault`, `showDefaultPromptPreview`,
  `isSystemPromptOverflowing`.
- Move derived state: `hasLoadedSystemPrompt`, `serverPrompt`, `serverIncludeDefault`,
  `defaultSystemPrompt`, `systemPromptDraft`, `includeDefaultDraft`, `systemInvisibleCharCount`,
  `isSystemPromptDirty`, `isSystemPromptDisabled`.
- Move handler: `handleSaveSystemPrompt`.
- Co-locate textarea constants (duplicate from PersonalInstructionsSettings) in this file.
- Duplicate `MutationCallbacks` interface in this file.
- Props: `systemPromptData`, `onSaveSystemPrompt`, `isSavingSystemPrompt`, `isSaveSystemPromptError`,
  `isAnyPromptSaving: boolean`.
- Remove system prompt props from parent's interface.
- Verify: `pnpm tsc --noEmit`, confirm `AdminWithDefaultToggle*` stories still render.

### Phase 4: Clean up parent

- `AgentSettingsBehaviorPageView` should now be a ~120-150-line layout shell.
- The parent's props interface passes props through to each child component.
- The `isAnyPromptSaving` computation (`isSavingSystemPrompt || isSavingUserPrompt`) stays
  in the parent and is passed to both prompt components.
- Remove all unused imports.
- Verify: `pnpm tsc --noEmit`.

### Phase 5: Final verification

- `cd site && pnpm tsc --noEmit` — no type errors.
- `cd site && pnpm check` or equivalent lint — no lint errors.
- Manually confirm stories render: `DesktopSetting`, `TogglesDesktop`, `AdminWithDefaultToggleOn`,
  `AdminWithDefaultToggleOff`, `DefaultAutostop*`.
- Confirm no behavioral changes apart from kyleosophy removal.
- Confirm `ChimeButton` stories still render (original chime preserved).
