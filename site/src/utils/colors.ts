/**
 * Does not support shorthand hex strings (e.g., #fff), just to maximize
 * compatibility with server, which also doesn't support shorthand
 */
const hexMatcher = /^#[0-9A-Fa-f]{6}$/;

/**
 * Determines whether a string is a hex color string. Returns false for
 * shorthand hex strings.
 *
 * Mainly here to validate input before sending it to the server.
 */
export function isHexColor(input: string): boolean {
  // Length check isn't necessary; it's just an fast way to validate before
  // kicking things off to the slower regex check
  return input.length === 7 && hexMatcher.test(input);
}

/**
 * Regex written and tested via Regex101. This doesn't catch every invalid HSL
 * string and still requires some other checks, but it can do a lot by itself.
 *
 * Setup:
 * - Supports capture groups for all three numeric values. Regex tries to fail
 *   the input as quickly as possible.
 * - Regex is all-or-nothing – there is some tolerance for extra spaces, but
 *   this regex will fail any string that is missing any part of the format.
 * - String is case-insensitive
 * - String must start with HSL and have both parentheses
 * - All three numeric values must be comma-delimited
 * - Hue can be 1-3 digits. Rightmost digit (if it exists) can only be 1-3;
 *   other digits have no constraints. The degree unit ("deg") is optional
 * - Both saturation and luminosity can be 1-3 digits. Rightmost digit (if it
 *   exists) can only ever be 1. Other digits have no constraints.
 */
const hslMatcher =
  /^hsl\(((?:[1-3]?\d)?\d)(?:deg)?, *((?:1?\d)?\d)%, *((?:1?\d)?\d)%\)$/i;

export function isHslColor(input: string): boolean {
  const [, hue, sat, lum] = hslMatcher.exec(input) ?? [];
  if (hue === undefined || sat === undefined || lum === undefined) {
    return false;
  }

  const hueN = Number(hue);
  if (!Number.isInteger(hueN) || hueN < 0 || hueN > 359) {
    return false;
  }

  const satN = Number(sat);
  if (!Number.isInteger(satN) || satN < 0 || satN > 100) {
    return false;
  }

  const lumN = Number(lum);
  if (!Number.isInteger(lumN) || lumN < 0 || lumN > 100) {
    return false;
  }

  return true;
}

// Used to convert our theme colors to Hex since monaco theme only support hex colors
// From https://www.jameslmilner.com/posts/converting-rgb-hex-hsl-colors/
export function hslToHex(hsl: string): string {
  const [h, s, l] = hsl
    .replace("hsl(", "")
    .replace(")", "")
    .replaceAll("%", "")
    .split(",")
    .map(Number);

  const hDecimal = l / 100;
  const a = (s * Math.min(hDecimal, 1 - hDecimal)) / 100;
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    const color = hDecimal - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);

    // Convert to Hex and prefix with "0" if required
    return Math.round(255 * color)
      .toString(16)
      .padStart(2, "0");
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}
