import { expect, type Page } from "@playwright/test";

type PollingOptions = { timeout?: number; intervals?: number[] };

export const expectUrl = expect.extend({
  /**
   * toHavePathName is an alternative to `toHaveURL` that won't fail if the URL contains query parameters.
   */
  async toHavePathName(page: Page, expected: string, options?: PollingOptions) {
    let actual: string = new URL(page.url()).pathname;
    let pass: boolean;
    try {
      await expect
        .poll(() => (actual = new URL(page.url()).pathname), options)
        .toBe(expected);
      pass = true;
    } catch {
      pass = false;
    }

    return {
      name: "toHavePathName",
      pass,
      actual,
      expected,
      message: () => "The page does not have the expected URL pathname.",
    };
  },
});
