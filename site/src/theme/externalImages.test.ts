import {
  forDarkThemes,
  forLightThemes,
  getExternalImageStylesFromUrl,
  parseImageParameters,
} from "./externalImages";

describe("externalImage parameters", () => {
  test("default parameters", () => {
    // Correctly selects default
    const widgetsStyles = getExternalImageStylesFromUrl(
      forDarkThemes,
      "/icon/widgets.svg",
    );
    expect(widgetsStyles).toBe(forDarkThemes.monochrome);

    // Allows overrides
    const overrideStyles = getExternalImageStylesFromUrl(
      forDarkThemes,
      "/icon/widgets.svg?fullcolor",
    );
    expect(overrideStyles).toBe(forDarkThemes.fullcolor);

    // Not actually a built-in
    const someoneElsesWidgetsStyles = getExternalImageStylesFromUrl(
      forDarkThemes,
      "https://example.com/icon/widgets.svg",
    );
    expect(someoneElsesWidgetsStyles).toBeUndefined();
  });

  test("blackWithColor brightness", () => {
    const tryCase = (params: string) =>
      parseImageParameters(forDarkThemes, params);

    const withDecimalValue = tryCase("?blackWithColor&brightness=1.5");
    expect(withDecimalValue?.filter).toBe(
      "invert(1) hue-rotate(180deg) brightness(1.5)",
    );

    const withPercentageValue = tryCase("?blackWithColor&brightness=150%");
    expect(withPercentageValue?.filter).toBe(
      "invert(1) hue-rotate(180deg) brightness(150%)",
    );

    // Sketchy `brightness` value will be ignored.
    const niceTry = tryCase(
      "?blackWithColor&brightness=</style><script>alert('leet hacking');</script>",
    );
    expect(niceTry?.filter).toBe("invert(1) hue-rotate(180deg)");

    const withLightTheme = parseImageParameters(
      forLightThemes,
      "?blackWithColor&brightness=1.5",
    );
    expect(withLightTheme).toBeUndefined();
  });

  test("whiteWithColor brightness", () => {
    const tryCase = (params: string) =>
      parseImageParameters(forLightThemes, params);

    const withDecimalValue = tryCase("?whiteWithColor&brightness=1.5");
    expect(withDecimalValue?.filter).toBe(
      "invert(1) hue-rotate(180deg) brightness(1.5)",
    );

    const withPercentageValue = tryCase("?whiteWithColor&brightness=150%");
    expect(withPercentageValue?.filter).toBe(
      "invert(1) hue-rotate(180deg) brightness(150%)",
    );

    // Sketchy `brightness` value will be ignored.
    const niceTry = tryCase(
      "?whiteWithColor&brightness=</style><script>alert('leet hacking');</script>",
    );
    expect(niceTry?.filter).toBe("invert(1) hue-rotate(180deg)");

    const withDarkTheme = parseImageParameters(
      forDarkThemes,
      "?whiteWithColor&brightness=1.5",
    );
    expect(withDarkTheme).toBeUndefined();
  });
});
