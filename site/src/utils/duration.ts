import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"

dayjs.extend(duration)
dayjs.extend(relativeTime)

export const humanDuration = (
  time: number,
  durationUnitType?: duration.DurationUnitType | undefined,
) => {
  return dayjs.duration(time, durationUnitType).humanize()
}
