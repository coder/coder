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
