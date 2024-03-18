import { intervalToDuration, formatDuration } from "date-fns";
import type { SerpentOption } from "api/typesGenerated";

// optionValue is a helper function to format the value of a specific deployment options
export function optionValue(
  option: SerpentOption,
  additionalValues?: string[],
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
      return (option.value as number).toString() + "s";
    case "OIDC Group Mapping":
      return Object.entries(option.value as Record<string, string>).map(
        ([key, value]) => `"${key}"->"${value}"`,
      );
    case "Experiments": {
      const experimentMap: Record<string, boolean> | undefined =
        additionalValues?.reduce(
          (acc, v) => {
            return { ...acc, [v]: option.value.includes("*") ? true : false };
          },
          {} as Record<string, boolean>,
        );

      if (!experimentMap) {
        break;
      }

      // We show all experiments (including unsafe) that are currently enabled on a deployment
      // but only show safe experiments that are not.
      for (const v of option.value) {
        if (v !== "*") {
          experimentMap[v] = true;
        }
      }

      return experimentMap;
    }
    default:
      return option.value;
  }
}
