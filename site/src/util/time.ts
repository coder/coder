export const getTimeSince = (date: Date): string => {
  const seconds = Math.floor((new Date().getTime() - date.getTime()) / 1000)
  let interval = seconds / 31536000

  const pluralize = (interval: number, text: string): string => {
    interval = Math.floor(interval)
    if (interval === 1) {
      return `${interval} ${text}`
    }
    return `${interval} ${text}s`
  }
  if (interval > 1) {
    return pluralize(interval, "year")
  }
  interval = seconds / 2592000
  if (interval > 1) {
    return pluralize(interval, "month")
  }
  interval = seconds / 86400
  if (interval > 1) {
    return pluralize(interval, "day")
  }
  interval = seconds / 3600
  if (interval > 1) {
    return pluralize(interval, "hour")
  }
  interval = seconds / 60
  if (interval > 1) {
    return pluralize(interval, "minute")
  }
  return pluralize(seconds, "second")
}
