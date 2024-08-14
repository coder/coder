/**
 * Not testing hslToHex, because it's code directly copied from a reliable
 * source
 */
import { isHslColor, isHexColor } from "./colors";

describe(`${isHslColor.name}`, () => {
  it("Should reject obviously wrong or malformed inputs", () => {
    const wrongInputs = [
      "",
      "Hi :)",
      "hsl",
      "#333",
      "#444666",
      "rgb(255, 300, 299)",
      "hsl(255deg, 10%, 20%",
      "hsv(255deg, 10%, 20%)",
      "hsb(255deg, 10%, 20%)",
      "hsl(0%, 10deg, 20)",
    ];

    for (const str of wrongInputs) {
      expect(isHslColor(str)).toBe(false);
    }
  });

  it("Should allow strings with or without deg unit", () => {
    const base = "hsl(200deg, 100%, 37%)";
    expect(isHslColor(base)).toBe(true);

    const withoutDeg = base.replace("deg", "");
    expect(isHslColor(withoutDeg)).toBe(true);
  });

  it("Should not care about whether there are spaces separating the inner values", () => {
    const inputs = [
      "hsl(180deg,20%,97%)",
      "hsl(180deg, 20%, 97%)",
      "hsl(180deg,           20%,         97%)",
    ];

    for (const str of inputs) {
      expect(isHslColor(str)).toBe(true);
    }
  });

  it("Should reject HSL strings that don't have percents for saturation/luminosity values", () => {
    expect(isHslColor("hsl(20%, 45, 92)")).toBe(false);
  });

  it("Should catch HSL strings that follow the correct pattern, but have impossible values", () => {
    const inputs = [
      "hsl(360deg, 120%, 240%)", // Impossible hue
      "hsl(100deg, 120%, 84%)", //  Impossible saturation
      "hsl(257, 12%, 110%)", //     Impossible luminosity
      "hsl(360deg, 120%, 240%)", // Impossible everything
    ];

    for (const str of inputs) {
      expect(isHslColor(str)).toBe(false);
    }
  });
});

describe(`${isHexColor.name}`, () => {
  it("Should reject obviously wrong or malformed inputs", () => {
    const inputs = ["", "#", "#bananas", "#ZZZZZZ", "AB179C", "#55555"];

    for (const str of inputs) {
      expect(isHexColor(str)).toBe(false);
    }
  });

  it("Should be fully case-insensitive", () => {
    const inputs = ["#aaaaaa", "#BBBBBB", "#CcCcCc"];

    for (const str of inputs) {
      expect(isHexColor(str)).toBe(true);
    }
  });
});
