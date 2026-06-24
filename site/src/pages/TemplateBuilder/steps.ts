import type { TemplateBuilderWizardState } from "./wizardState";

export type StepId =
	| "base-infra"
	| "base-parameters"
	| "module-select"
	| "module-settings"
	| "customizations";

type WizardStep = {
	id: StepId;
	/** Maps to one of the three sidebar groups in SelectionSummary. */
	group: 1 | 2 | 3;
	/** Return true to skip this step during navigation. */
	shouldSkip: (state: TemplateBuilderWizardState) => boolean;
};

/**
 * Returns true when at least one selected module exposes variables that
 * need user configuration (non-sensitive).
 */
export const stepModuleSettingsRequired = (
	state: TemplateBuilderWizardState,
): boolean => {
	return state.selectedModules.some((m) => m.hasConfigurableVars);
};

/**
 * Ordered registry of all wizard steps.
 *
 * Invariant: a step must not edit the state slice that controls its own
 * shouldSkip predicate.
 */
export const WIZARD_STEPS: readonly WizardStep[] = [
	{
		id: "base-infra",
		group: 1,
		shouldSkip: () => false,
	},
	{
		id: "base-parameters",
		group: 1,
		shouldSkip: (state) =>
			!state.selectedBase?.hasParameters &&
			!state.selectedBase?.hasPrerequisites,
	},
	{
		id: "module-select",
		group: 2,
		shouldSkip: () => false,
	},
	{
		id: "module-settings",
		group: 2,
		shouldSkip: (state) => !stepModuleSettingsRequired(state),
	},
	{
		id: "customizations",
		group: 3,
		shouldSkip: () => false,
	},
];

/**
 * Find the next visible (non-skipped) step index starting after `fromIndex`.
 * Returns -1 if no visible step exists ahead.
 */
export function findNextVisibleIndex(
	fromIndex: number,
	state: TemplateBuilderWizardState,
): number {
	for (let i = fromIndex + 1; i < WIZARD_STEPS.length; i++) {
		if (!WIZARD_STEPS[i].shouldSkip(state)) {
			return i;
		}
	}
	return -1;
}

/**
 * Find the previous visible (non-skipped) step index before `fromIndex`.
 * Returns -1 if no visible step exists behind.
 */
export function findPrevVisibleIndex(
	fromIndex: number,
	state: TemplateBuilderWizardState,
): number {
	for (let i = fromIndex - 1; i >= 0; i--) {
		if (!WIZARD_STEPS[i].shouldSkip(state)) {
			return i;
		}
	}
	return -1;
}

/**
 * Given an index that may point to a skipped step, find the nearest
 * visible step. Searches backward first, then forward.
 */
export function nearestVisible(
	index: number,
	state: TemplateBuilderWizardState,
): number {
	// Handle skipped steps
	if (index >= 0 && index < WIZARD_STEPS.length) {
		if (!WIZARD_STEPS[index].shouldSkip(state)) {
			return index;
		}
	}
	// Search backward.
	for (let i = index - 1; i >= 0; i--) {
		if (!WIZARD_STEPS[i].shouldSkip(state)) {
			return i;
		}
	}
	// Search forward.
	for (let i = index + 1; i < WIZARD_STEPS.length; i++) {
		if (!WIZARD_STEPS[i].shouldSkip(state)) {
			return i;
		}
	}
	return 0;
}
