/**
 * This test is for all useClipboard functionality, with the browser context
 * set to insecure (HTTP connections).
 *
 * See useClipboard.test-setup.ts for more info on why this file is set up the
 * way that it is.
 */
import { useClipboard } from "./useClipboard";
import { scheduleClipboardTests } from "./useClipboard.test-setup";

describe(useClipboard.name, () => {
  describe("HTTP (non-secure) connections", () => {
    scheduleClipboardTests({ isHttps: false });
  });
});
