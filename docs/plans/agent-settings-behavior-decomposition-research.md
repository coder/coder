# Research: AgentSettingsBehaviorPageView Decomposition

## Problem Context

`AgentSettingsBehaviorPageView` (717 lines) is a monolithic component managing 7 visual settings sections
(6 unextracted), each with its own local state, derived state, and event handlers. Danielle's review on
[PR #23833](https://github.com/coder/coder/pull/23833#pullrequestreview-4073859981) identified this:

> "The AgentSettingsBehaviorPage has a lot of forms and I think we're definitely at the point where we need
> to use components instead of having a god component"

The props interface alone has **30+ props**, most of which are scoped to individual sections. Each new
setting added increases complexity non-linearly: the shared render function, growing local state surface,
and the expanding props interface mean each addition costs more than the last.

### Current Sections

| # | Section               | Lines   | Local State Vars                                                                                | Admin-Only | Has Form    |
|---|-----------------------|---------|-------------------------------------------------------------------------------------------------|------------|-------------|
| 1 | Personal Instructions | 327-384 | 2 (`localUserEdit`, `isUserPromptOverflowing`)                                                  | No         | Yes         |
| 2 | Compaction Thresholds | 387-396 | 0 (already extracted)                                                                           | No         | N/A         |
| 3 | System Instructions   | 398-484 | 4 (`localEdit`, `localIncludeDefault`, `showDefaultPromptPreview`, `isSystemPromptOverflowing`) | Yes        | Yes         |
| 4 | Virtual Desktop       | 486-527 | 0                                                                                               | Yes        | No (toggle) |
| 5 | Workspace Autostop    | 529-595 | 2 (`localTTLMs`, `autostopToggled`)                                                             | Yes        | Yes         |
| 6 | Retention Period      | 597-678 | 2 (`localRetentionDays`, `retentionToggled`)                                                    | Yes        | Yes         |
| 7 | Kyleosophy            | 682-706 | 1 (`kylesophyEnabled`) + `kylesophyForced` constant                                             | No         | No (toggle) |

### Cross-Section Coupling

There is **one coupling** between sections:

```typescript
// Line 178
const isPromptSaving = isSavingSystemPrompt || isSavingUserPrompt;
```

This disables the Personal Instructions textarea while the System Instructions are saving, and vice versa.
This is used in:

- Personal Instructions: `disabled={isPromptSaving}` (textarea), `disabled={isPromptSaving || !userPromptDraft}` (Clear), `disabled={isPromptSaving || !isUserPromptDirty}` (Save)
- System Instructions: `isSystemPromptDisabled = isPromptSaving || !hasLoadedSystemPrompt` (textarea, Clear, Save)

Note: `isPromptSaving` is the **combined** saving state of both prompts, not just "the other" prompt's
state. Both components receive and are disabled by the same flag — including their own saving state.

The `TextPreviewDialog` (line 707-713) is rendered at root level but driven by `showDefaultPromptPreview`
state from the System Instructions section.

### Existing Precedent: UserCompactionThresholdSettings

`UserCompactionThresholdSettings` (~405 lines, `components/UserCompactionThresholdSettings.tsx`) is already
extracted as a sub-component. It demonstrates the project pattern:

- Self-contained component with own props interface
- Lives in `components/` directory
- Has dedicated `.stories.tsx` file
- Owns all its local state and derived state
- Parent passes data + callbacks

```typescript
// From UserCompactionThresholdSettings.tsx lines 23-35
interface UserCompactionThresholdSettingsProps {
    modelConfigs: readonly TypesGen.ChatModelConfig[];
    modelConfigsError?: unknown;
    isLoadingModelConfigs?: boolean;
    thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
    isThresholdsLoading: boolean;
    thresholdsError: unknown;
    onSaveThreshold: (modelConfigId: string, thresholdPercent: number) => Promise<unknown>;
    onResetThreshold: (modelConfigId: string) => Promise<unknown>;
}
```

### Shared UI Constants

Two constants are shared across sections:

```typescript
// Line 22-25
const textareaMaxHeight = 240;
const textareaBaseClassName = "max-h-[240px] w-full resize-none rounded-lg border ...";
const textareaOverflowClassName = "overflow-y-auto [scrollbar-width:thin]";
```

These are used only by Personal Instructions and System Instructions (both textarea forms).

### MutationCallbacks Type

```typescript
// Lines 27-30
interface MutationCallbacks {
    onSuccess?: () => void;
    onError?: () => void;
}
```

Used by all `onSave*` handlers. Shared across sections.

### Existing Stories Coverage Gap

The current `AgentSettingsBehaviorPageView.stories.tsx` has **no retention period coverage**. The
`baseProps` and `meta.args` objects are missing `retentionDaysData`, `onSaveRetentionDays`,
`isSavingRetentionDays`, and `isSaveRetentionDaysError`. This is a gap that should be filled during
the decomposition.

## Approaches

### Approach A: Extract All Sections as Independent Components

**Axis:** Maximum decomposition — each visual section becomes its own component.

**Description:** Extract all 6 unextracted sections (Personal Instructions, System Instructions,
Virtual Desktop, Workspace Autostop, Retention Period, Kyleosophy) into separate component files
in `components/`. The parent `AgentSettingsBehaviorPageView` becomes a layout shell that renders
`<SectionHeader>`, section components with `<hr>` dividers, and manages the admin-only visibility gate.

**Precedent:** `UserCompactionThresholdSettings` — already extracted, same pattern.

**Strongest argument for:** Maximally composable. Each section is independently testable via Storybook.
Adding new settings sections doesn't increase the complexity of existing ones. The parent becomes a
~120-150-line layout shell (imports, slimmed props interface, JSX with prop threading and dividers).

**Strongest argument against:** The `isPromptSaving` coupling between Personal Instructions and System
Instructions means those two components can't be fully independent — one needs to know the other's saving
state. Requires either lifting that state to the parent or accepting a prop for it.

**What this makes easy:** Adding new settings, testing sections in isolation, code review of individual
sections.

**What this makes hard:** Nothing significant. The coupling is manageable with a simple prop.

### Approach B: Group Prompts Together, Extract Admin Sections Separately

**Axis:** Coupling-aware grouping — keep coupled sections together, extract independent ones.

**Description:** Create a `PromptSettings` component containing both Personal Instructions and System
Instructions (since they share `isPromptSaving`). Extract Virtual Desktop, Workspace Autostop, Retention
Period, and Kyleosophy as separate components.

**Precedent:** No direct precedent in the codebase for this grouping pattern.

**Strongest argument for:** Avoids the cross-component coupling entirely. The `isPromptSaving` logic stays
internal to one component.

**Strongest argument against:** The prompt component would still be ~200 lines with 6 local state
variables and 10+ props. It doesn't fully solve the god component problem — it just makes a smaller
god component. The Personal and System prompts are conceptually distinct (user vs. admin) and only coupled
by a UI decision to disable concurrently.

**What this makes easy:** No prop threading for `isPromptSaving`.

**What this makes hard:** Testing the personal prompt in isolation. The combined component still has two
forms interleaved. Adding a new prompt type would grow this combined component.

### Approach C: Extract Only Large Sections, Inline Small Toggles

**Axis:** Size-based threshold — extract sections above a complexity threshold, leave simple toggles inline.

**Description:** Extract Personal Instructions, System Instructions, Workspace Autostop, and Retention
Period (all have local state + forms). Leave Virtual Desktop and Kyleosophy inline since they're
simple toggles with no local form state.

**Precedent:** The current state of the file — Kyleosophy and Virtual Desktop are already simple enough
to read inline.

**Strongest argument for:** Pragmatic. Doesn't create tiny 20-line components that add indirection without
meaningful benefit.

**Strongest argument against:** Inconsistent. Future settings additions won't have a clear rule for
"extract or inline." The parent still knows about desktop and kyleosophy state. Every section being
extracted creates a uniform pattern.

**What this makes easy:** Less file churn for minimal-complexity sections.

**What this makes hard:** Deciding the threshold. The Virtual Desktop section (42 lines) is arguable
either way.

## Decisions

1. **Approach: Extract all sections (A).** Human ratified agent recommendation.
   Human said: "Agreed with A." Agent recommended Approach A for maximum decomposition
   following the `UserCompactionThresholdSettings` precedent.

2. **Preserve `isPromptSaving` coupling.** Human decision.
   Human said: "For simplicity let's preserve the coupling. Right now we have all of these
   settings under multiple endpoints, but I think at some later point we may consolidate
   them to one single endpoint." Pass `isAnyPromptSaving` to both prompt components.

3. **Duplicate `MutationCallbacks` per component.** Human decision.
   Human said: "Let's duplicate for now and see what that looks like before refactoring."

4. **Co-locate textarea constants with prompt components.** Human decision.
   Human said: "Let's colocate for now."

5. **Keep existing stories unchanged.** Human decision.
   Human said: "Let's keep the existing stories; this is just a refactor."
   No new per-component stories. Existing parent stories serve as regression tests.

## Open Questions

None remaining. All decisions locked in.
