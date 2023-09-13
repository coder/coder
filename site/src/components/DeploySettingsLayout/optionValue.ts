import { DeploymentOption } from "api/api";
import { intervalToDuration, formatDuration } from "date-fns";

// optionValue is a helper function to format the value of a specific deployment options
export function optionValue(option: DeploymentOption) {
  switch (option.name) {
    case "Max Token Lifetime":
    case "Session Duration":
      // intervalToDuration takes ms, so convert nanoseconds to ms
      return formatDuration(
        intervalToDuration({ start: 0, end: (option.value as number) / 1e6 }),
      );
    case "Strict-Transport-Security":
      if (option.value === 0) {
        return "Disabled";
      }
      return (option.value as number).toString() + "s";
    case "OIDC Group Mapping":
      return Object.entries(option.value as Record<string, string>).map(
        ([key, value]) => `"${key}"->"${value}"`,
      );
    default:
      return option.value;
  }
}
