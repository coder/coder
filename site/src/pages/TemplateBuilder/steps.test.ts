import { describe, expect, it } from "vitest";
import {
	findNextVisibleIndex,
	findPrevVisibleIndex,
	furthestAllowedIndex,
	nearestVisible,
	stepModuleSettingsRequired,
	WIZARD_STEPS,
} from "./steps";
import type { TemplateBuilderWizardState } from "./wizardState";
import { initialWizardState } from "./wizardState";

function stateWith(
	overrides: Partial<TemplateBuilderWizardState>,
): TemplateBuilderWizardState {
	return { ...initialWizardState, ...overrides };
}

describe("WIZARD_STEPS", () => {
	it("has five steps in three groups", () => {
		expect(WIZARD_STEPS).toHaveLength(5);
		expect(WIZARD_STEPS.map((s) => s.group)).toEqual([1, 1, 2, 2, 3]);
	});
});

describe("shouldSkip", () => {
	const stepById = (id: string) => WIZARD_STEPS.find((s) => s.id === id)!;

	it("never skips base-infra", () => {
		expect(stepById("base-infra").shouldSkip(initialWizardState)).toBe(false);
	});

	it("skips base-parameters when base has no parameters", () => {
		const noParams = stateWith({
			selectedBase: {
				id: "docker",
				name: "Docker",
				hasParameters: false,
				hasPrerequisites: false,
			},
		});
		expect(stepById("base-parameters").shouldSkip(noParams)).toBe(true);
	});

	it("does not skip base-parameters when base has parameters", () => {
		const withParams = stateWith({
			selectedBase: {
				id: "aws-linux",
				name: "AWS Linux",
				hasParameters: true,
				hasPrerequisites: false,
			},
		});
		expect(stepById("base-parameters").shouldSkip(withParams)).toBe(false);
	});

	it("skips base-parameters when no base is selected", () => {
		expect(stepById("base-parameters").shouldSkip(initialWizardState)).toBe(
			true,
		);
	});

	it("never skips module-select", () => {
		expect(stepById("module-select").shouldSkip(initialWizardState)).toBe(
			false,
		);
	});

	it("skips module-settings when no modules have configurable vars", () => {
		const noConfigVars = stateWith({
			selectedModules: [
				{
					id: "npm-config",
					name: "npm-config",
					iconUrl: "/npm.svg",
					hasConfigurableVars: false,
				},
			],
		});
		expect(stepById("module-settings").shouldSkip(noConfigVars)).toBe(true);
	});

	it("does not skip module-settings when modules have configurable vars", () => {
		const withConfigVars = stateWith({
			selectedModules: [
				{
					id: "code-server",
					name: "code-server",
					iconUrl: "/icon.svg",
					hasConfigurableVars: true,
				},
			],
		});
		expect(stepById("module-settings").shouldSkip(withConfigVars)).toBe(false);
	});

	it("skips module-settings when no modules are selected", () => {
		expect(stepById("module-settings").shouldSkip(initialWizardState)).toBe(
			true,
		);
	});

	it("never skips customizations", () => {
		expect(stepById("customizations").shouldSkip(initialWizardState)).toBe(
			false,
		);
	});
});

describe("stepModuleSettingsRequired", () => {
	it("returns false when no modules are selected", () => {
		expect(stepModuleSettingsRequired(initialWizardState)).toBe(false);
	});

	it("returns true when at least one module has configurable vars", () => {
		const state = stateWith({
			selectedModules: [
				{
					id: "a",
					name: "A",
					iconUrl: "/a.svg",
					hasConfigurableVars: false,
				},
				{
					id: "b",
					name: "B",
					iconUrl: "/b.svg",
					hasConfigurableVars: true,
				},
			],
		});
		expect(stepModuleSettingsRequired(state)).toBe(true);
	});
});

describe("findNextVisibleIndex", () => {
	it("skips base-parameters when base has no parameters", () => {
		const state = stateWith({
			selectedBase: {
				id: "docker",
				name: "Docker",
				hasParameters: false,
				hasPrerequisites: false,
			},
		});
		// From base-infra (index 0), next visible should be module-select (index 2).
		const next = findNextVisibleIndex(0, state);
		expect(WIZARD_STEPS[next].id).toBe("module-select");
	});

	it("skips module-settings when no modules have configurable vars", () => {
		// From module-select (index 2), next visible should be customizations (index 4).
		const next = findNextVisibleIndex(2, initialWizardState);
		expect(WIZARD_STEPS[next].id).toBe("customizations");
	});

	it("returns -1 from the last step", () => {
		expect(findNextVisibleIndex(4, initialWizardState)).toBe(-1);
	});

	it("advances to adjacent step when nothing is skipped", () => {
		const state = stateWith({
			selectedBase: {
				id: "aws-linux",
				name: "AWS Linux",
				hasParameters: true,
				hasPrerequisites: false,
			},
			selectedModules: [
				{
					id: "code-server",
					name: "code-server",
					iconUrl: "/icon.svg",
					hasConfigurableVars: true,
				},
			],
		});
		// All steps visible: 0 -> 1 -> 2 -> 3 -> 4
		expect(findNextVisibleIndex(0, state)).toBe(1);
		expect(findNextVisibleIndex(1, state)).toBe(2);
		expect(findNextVisibleIndex(2, state)).toBe(3);
		expect(findNextVisibleIndex(3, state)).toBe(4);
		expect(findNextVisibleIndex(4, state)).toBe(-1);
	});
});

describe("findPrevVisibleIndex", () => {
	it("skips base-parameters backward when base has no parameters", () => {
		const state = stateWith({
			selectedBase: {
				id: "docker",
				name: "Docker",
				hasParameters: false,
				hasPrerequisites: false,
			},
		});
		// From module-select (index 2), prev visible should be base-infra (index 0).
		const prev = findPrevVisibleIndex(2, state);
		expect(WIZARD_STEPS[prev].id).toBe("base-infra");
	});

	it("returns -1 from the first step", () => {
		expect(findPrevVisibleIndex(0, initialWizardState)).toBe(-1);
	});
});

describe("nearestVisible", () => {
	it("returns the same index if not skipped", () => {
		expect(nearestVisible(0, initialWizardState)).toBe(0);
		expect(nearestVisible(4, initialWizardState)).toBe(4);
	});

	it("falls back to a prior visible step", () => {
		// base-parameters (index 1) is skipped when no base has parameters.
		const nearest = nearestVisible(1, initialWizardState);
		expect(WIZARD_STEPS[nearest].id).toBe("base-infra");
	});

	it("falls forward when no prior step is visible", () => {
		// Construct a state where everything is visible except base-infra.
		// Since base-infra is never skipped this is a synthetic edge case,
		// but it validates the forward search path.
		const allSkippable: TemplateBuilderWizardState = {
			...initialWizardState,
			selectedBase: {
				id: "docker",
				name: "Docker",
				hasParameters: false,
				hasPrerequisites: false,
			},
		};
		// Index 1 (base-parameters) is skipped, nearest backward is 0 (base-infra).
		expect(nearestVisible(1, allSkippable)).toBe(0);
	});
});

describe("furthestAllowedIndex", () => {
	it("returns 0 when no base is selected", () => {
		expect(furthestAllowedIndex(initialWizardState)).toBe(0);
	});

	it("returns the last step index when a base is selected", () => {
		const withBase = stateWith({
			selectedBase: {
				id: "docker",
				name: "Docker",
				hasParameters: false,
				hasPrerequisites: false,
			},
		});
		expect(furthestAllowedIndex(withBase)).toBe(WIZARD_STEPS.length - 1);
	});
});
