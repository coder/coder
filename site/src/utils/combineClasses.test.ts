import { combineClasses } from "./combineClasses";

const staticStyles = {
  text: "MuiText",
  success: "MuiText-Green",
  warning: "MuiText-Red",
};

describe("combineClasses", () => {
  it.each([
    // Falsy
    [undefined, undefined],
    [{ [staticStyles.text]: false }, undefined],
    [{ [staticStyles.text]: undefined }, undefined],
    [[], undefined],

    // Truthy
    [{ [staticStyles.text]: true }, "MuiText"],
    [
      { [staticStyles.text]: true, [staticStyles.warning]: true },
      "MuiText  MuiText-Red",
    ],
    [[staticStyles.text], "MuiText"],

    // Mixed
    [{ [staticStyles.text]: true, [staticStyles.success]: false }, "MuiText"],
    [[staticStyles.text, staticStyles.success], "MuiText  MuiText-Green"],
  ])(`classNames(%p) returns %p`, (staticClasses, result) => {
    expect(combineClasses(staticClasses)).toBe(result);
  });
});
