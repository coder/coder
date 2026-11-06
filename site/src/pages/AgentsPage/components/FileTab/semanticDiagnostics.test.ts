import { describe, expect, it } from "vitest";
import { createSemanticDiagnosticsSuppressor } from "./semanticDiagnostics";

type Options = {
	noSemanticValidation?: boolean;
	noSyntaxValidation?: boolean;
	diagnosticCodesToIgnore?: number[];
};

const createDefaults = (initial: Options) => {
	let options = initial;
	return {
		getDiagnosticsOptions: () => options,
		setDiagnosticsOptions: (next: Options) => {
			options = next;
		},
	};
};

describe("createSemanticDiagnosticsSuppressor", () => {
	it("turns semantic checks off until the last holder releases, then restores the earlier options", () => {
		const earlier = {
			noSemanticValidation: false,
			noSyntaxValidation: false,
			diagnosticCodesToIgnore: [2307],
		};
		const defaults = createDefaults(earlier);
		const suppress = createSemanticDiagnosticsSuppressor(defaults);

		const releaseFirst = suppress();
		const releaseSecond = suppress();
		expect(defaults.getDiagnosticsOptions()).toEqual({
			...earlier,
			noSemanticValidation: true,
		});

		releaseFirst();
		releaseFirst();
		expect(defaults.getDiagnosticsOptions().noSemanticValidation).toBe(true);

		releaseSecond();
		expect(defaults.getDiagnosticsOptions()).toEqual(earlier);
	});
});
