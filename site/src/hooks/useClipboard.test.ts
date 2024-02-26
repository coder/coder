import { type UseClipboardResult, useClipboard } from "./useClipboard";
import { act, renderHook } from "@testing-library/react";

/*
  Normally, you could call userEvent.setup to enable clipboard mocking, but
  userEvent doesn't expose a teardown function. It also modifies the global
  clipboard, so enabling just one userEvent session will make a mock clipboard
  exist for all other tests, even though you didn't tell them to set up a
  session. The mock also assumes that the clipboard API will always be
  available, which is not true on HTTP-only connections

  Since these tests need to split hairs and differentiate between HTTP and HTTPS
  connections, setting up a single userEvent is disastrous. It will make all the
  tests pass, even if they shouldn't. Have to avoid that by creating a custom
  clipboard mock.
*/
type MockClipboard = Readonly<
  Clipboard & {
    resetText: () => void;
    setIsSecureContext: (newContext: boolean) => void;
  }
>;

function makeMockClipboard(): MockClipboard {
  let mockClipboardValue = "";
  let isSecureContext = true;

  return {
    readText: async () => {
      if (!isSecureContext) {
        throw new Error(
          "Trying to read from clipboard outside secure context!",
        );
      }

      return mockClipboardValue;
    },
    writeText: async (newText) => {
      if (!isSecureContext) {
        throw new Error("Trying to write to clipboard outside secure context!");
      }

      mockClipboardValue = newText;
    },
    resetText: () => {
      mockClipboardValue = "";
    },
    setIsSecureContext: (newContext) => {
      isSecureContext = newContext;
    },

    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
    read: jest.fn(),
    write: jest.fn(),
  };
}

const mockClipboard = makeMockClipboard();

beforeAll(() => {
  const originalNavigator = window.navigator;
  jest.spyOn(window, "navigator", "get").mockImplementation(() => ({
    ...originalNavigator,
    clipboard: mockClipboard,
  }));

  jest.spyOn(document, "hasFocus").mockImplementation(() => true);
  jest.useFakeTimers();
});

afterEach(() => {
  mockClipboard.resetText();
});

afterAll(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
});

function renderUseClipboard(textToCopy: string) {
  type Props = Readonly<{ textToCopy: string }>;
  return renderHook<UseClipboardResult, Props>(
    ({ textToCopy }) => useClipboard(textToCopy),
    { initialProps: { textToCopy } },
  );
}

type UseClipboardTestResult = ReturnType<typeof renderUseClipboard>["result"];

async function assertClipboardTextUpdate(
  result: UseClipboardTestResult,
  textToCheck: string,
): Promise<void> {
  await act(() => result.current.copyToClipboard());
  expect(result.current.showCopiedSuccess).toBe(true);

  const clipboardText = await window.navigator.clipboard.readText();
  expect(textToCheck).toEqual(clipboardText);
}

function scheduleTests(isHttps: boolean) {
  mockClipboard.setIsSecureContext(isHttps);

  it("Copies the current text to the user's clipboard", async () => {
    const hookText = "dogs";
    const { result } = renderUseClipboard(hookText);
    await assertClipboardTextUpdate(result, hookText);
  });

  it("Should indicate to components not to show successful copy after a set period of time", async () => {
    const hookText = "cats";
    const { result } = renderUseClipboard(hookText);
    await assertClipboardTextUpdate(result, hookText);

    setTimeout(() => {
      expect(result.current.showCopiedSuccess).toBe(false);
    }, 10_000);

    await jest.runAllTimersAsync();
  });
}

describe(useClipboard.name, () => {
  describe("HTTP (non-secure) connections", () => {
    scheduleTests(false);
  });

  describe("HTTPS connections", () => {
    scheduleTests(true);
  });
});
