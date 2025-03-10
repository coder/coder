import type { SerpentOption } from "api/typesGenerated";
import { formatDuration, intervalToDuration } from "date-fns";

// optionValue is a helper function to format the value of a specific deployment options
export function optionValue(
	option: SerpentOption,
	additionalValues?: readonly string[],
) {
	// If option annotations are present, use them to format the value.
	if (option.annotations) {
		for (const [k, v] of Object.entries(option.annotations)) {
			if (v !== "true") {
				continue; // skip if not explicitly true
			}
			switch (k) {
				case "format_duration":
					return formatDuration(
						// intervalToDuration takes ms, so convert nanoseconds to ms
						intervalToDuration({
							start: 0,
							end: (option.value as number) / 1e6,
						}),
					);
				// Add additional cases here as needed.
			}
		}
	}

	// If no format annotations are present, use the option name to format the value.
	switch (option.name) {
		case "Strict-Transport-Security":
			if (option.value === 0) {
				return "Disabled";
			}
			return `${(option.value as number).toString()}s`;
		case "OIDC Group Mapping":
			return Object.entries(option.value as Record<string, string>).map(
				([key, value]) => `"${key}"->"${value}"`,
			);
		case "Experiments": {
			const experimentMap = additionalValues?.reduce<Record<string, boolean>>(
				(acc, v) => {
					// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
					acc[v] = (option.value as any).includes("*");
					return acc;
				},
				{},
			);

			if (!experimentMap) {
				break;
			}

			if (!option.value) {
				return "";
			}

			// We show all experiments (including unsafe) that are currently enabled on a deployment
			// but only show safe experiments that are not.
			// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
			for (const v of option.value as any) {
				if (v !== "*") {
					experimentMap[v] = true;
				}
			}
			return experimentMap;
		}
		default:
			// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
			return option.value as any;
	}
}
