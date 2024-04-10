import { expect, type Page, waitFor } from "@playwright/test";

export const expectUrl = expect.extend({
  async toHavePath(page: Page, pathname: string) {
    let url;
    let pass;

    try {
      await waitFor(() => {
        url = new URL(page.url());
        expect(url.pathname).toBe(pathname);
      });
      pass = true;
    } catch {
      pass = false;
    }

    return {
      message: () => "foob",
      pass,
      name: "toHavePath",
      expected: pathname,
      actual: url.toString(),
    };
  },
});
