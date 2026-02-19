import type { SerpentOption } from "api/typesGenerated";

export interface ConfigWarning {
	option: string;
	severity: "error" | "warning";
	message: string;
}

export interface ConfigContext {
	isHTTPS: boolean;
}

/**
 * Computes configuration warnings by parsing annotations on deployment options.
 * Annotations follow the format:
 * - Key: "<condition>_<severity>_<hash>" (e.g., "when_https_require_a1b2c3d4")
 * - Value: "<expected>|<message>" (e.g., "true|Secure cookies should be enabled...")
 */
export function computeConfigWarnings(
	options: readonly SerpentOption[],
	context: ConfigContext,
): ConfigWarning[] {
	const warnings: ConfigWarning[] = [];

	for (const opt of options) {
		if (!opt.annotations) continue;

		for (const [key, value] of Object.entries(opt.annotations)) {
			const warning = checkAnnotation(key, value, opt, context);
			if (warning) {
				warnings.push(warning);
			}
		}
	}

	return warnings;
}

function checkAnnotation(
	key: string,
	value: string,
	opt: SerpentOption,
	context: ConfigContext,
): ConfigWarning | null {
	// Parse annotation key: "<condition>_<severity>_<hash>"
	let severity: "error" | "warning";
	let conditionApplies: boolean;

	if (key.startsWith("when_https_suggest_")) {
		severity = "warning";
		conditionApplies = context.isHTTPS;
	} else if (key.startsWith("when_https_require_")) {
		severity = "error";
		conditionApplies = context.isHTTPS;
	} else {
		return null; // Not a config recommendation annotation
	}

	if (!conditionApplies) {
		return null;
	}

	// Parse value: "<expected>|<message>"
	const separatorIndex = value.indexOf("|");
	if (separatorIndex === -1) {
		return null;
	}

	const expected = value.slice(0, separatorIndex);
	const message = value.slice(separatorIndex + 1);

	// Get current value
	const currentValue = String(opt.value ?? "");

	// Check if current value matches expected
	if (checkValueMatches(currentValue, expected)) {
		return null;
	}

	return {
		option: opt.flag ?? opt.name ?? "unknown",
		severity,
		message,
	};
}

/**
 * Checks if a current value matches an expected value.
 * Supports simple comparisons and operators like ">0".
 */
function checkValueMatches(current: string, expected: string): boolean {
	if (expected.startsWith(">")) {
		const threshold = Number.parseInt(expected.slice(1), 10);
		if (Number.isNaN(threshold)) {
			return true; // Can't parse, assume OK
		}
		const val = Number.parseInt(current, 10);
		if (Number.isNaN(val)) {
			return true;
		}
		return val > threshold;
	}
	return current.toLowerCase() === expected.toLowerCase();
}
