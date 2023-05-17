import { Theme } from "@mui/material/styles"

// Used to convert our theme colors to Hex since monaco theme only support hex colors
// From https://www.jameslmilner.com/posts/converting-rgb-hex-hsl-colors/
export function hslToHex(hsl: string): string {
  const [h, s, l] = hsl
    .replace("hsl(", "")
    .replace(")", "")
    .replaceAll("%", "")
    .split(",")
    .map(Number)

  const hDecimal = l / 100
  const a = (s * Math.min(hDecimal, 1 - hDecimal)) / 100
  const f = (n: number) => {
    const k = (n + h / 30) % 12
    const color = hDecimal - a * Math.max(Math.min(k - 3, 9 - k, 1), -1)

    // Convert to Hex and prefix with "0" if required
    return Math.round(255 * color)
      .toString(16)
      .padStart(2, "0")
  }
  return `#${f(0)}${f(8)}${f(4)}`
}

// getLatencyColor is the text color to use for a given latency
// in milliseconds.
export const getLatencyColor = (theme: Theme, latency: number) => {
  let color = theme.palette.success.light
  if (latency >= 150 && latency < 300) {
    color = theme.palette.warning.light
  } else if (latency >= 300) {
    color = theme.palette.error.light
  }
  return color
}
