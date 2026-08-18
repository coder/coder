import { modelConfigReasoningEffortStorage } from "#/utils/storage/keys";

/** Reads the persisted effort for a model, or undefined when none is stored or storage is unavailable. */
export const getReasoningEffortForModel = (
	modelID: string,
): string | undefined =>
	modelConfigReasoningEffortStorage.forId(modelID).get() ?? undefined;

/** Persists the effort for a model. Storage errors (private mode, quota) fail soft so the caller's in-memory selection is unaffected. */
export const saveReasoningEffortForModel = (
	modelID: string,
	reasoningEffort: string,
): void => {
	modelConfigReasoningEffortStorage.forId(modelID).set(reasoningEffort);
};

/** Display label for an effort value, e.g. "xhigh" renders as "Xhigh". */
export const formatReasoningEffort = (value: string): string =>
	value.charAt(0).toUpperCase() + value.slice(1);

/** Chooses requested effort, then default effort, then the last selectable effort. */
export const pickReasoningEffort = (
	value: string | undefined,
	efforts: readonly string[],
	defaultValue?: string,
): string | undefined => {
	if (efforts.length === 0) {
		return undefined;
	}

	if (value && efforts.includes(value)) {
		return value;
	}

	if (defaultValue && efforts.includes(defaultValue)) {
		return defaultValue;
	}

	return efforts[efforts.length - 1];
};
