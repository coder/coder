import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"

dayjs.extend(duration)

export const formatTemplateActiveDevelopers = (num?: number): string => {
  if (num === undefined || num < 0) {
    // Loading
    return "-"
  }
  return num.toString()
}

export const formatTemplateBuildTime = (buildTimeMs: number): string => {
  return buildTimeMs < 0
    ? "Unknown"
    : dayjs.duration(buildTimeMs, "milliseconds").humanize()
}
