/*
  Normally, you could call userEvent.setup to enable clipboard mocking, but
  userEvent doesn't expose a teardown function. It also modifies the global
  scope for the whole test file, so enabling just one userEvent session will
  make a mock clipboard exist for all other tests, even though you didn't tell
  them to set up a session. The mock also assumes that the clipboard API will
  always be available, which is not true on HTTP-only connections

  Since these tests need to split hairs and differentiate between HTTP and HTTPS
  connections, setting up a single userEvent is disastrous. It will make all the
  tests pass, even if they shouldn't. Have to avoid that by creating a custom
  clipboard mock.
*/
import { type UseClipboardResult, useClipboard } from "./useClipboard";
import { act, renderHook } from "@testing-library/react";

const initialExecCommand = global.document.execCommand;
beforeAll(() => {
  jest.useFakeTimers();
});

afterAll(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
  global.document.execCommand = initialExecCommand;
});

type MockClipboardEscapeHatches = Readonly<{
  getMockText: () => string;
  setMockText: (newText: string) => void;
}>;

type MockClipboard = Readonly<Clipboard & MockClipboardEscapeHatches>;
function makeMockClipboard(isSecureContext: boolean): MockClipboard {
  let mockClipboardValue = "";

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

    getMockText: () => mockClipboardValue,
    setMockText: (newText) => {
      mockClipboardValue = newText;
    },

    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
    read: jest.fn(),
    write: jest.fn(),
  };
}

function renderUseClipboard(textToCopy: string) {
  return renderHook<UseClipboardResult, { hookText: string }>(
    ({ hookText }) => useClipboard(hookText),
    { initialProps: { hookText: textToCopy } },
  );
}

/**
 * Unconventional test setup, but we need two separate instances of the
 * MockClipboard (one for HTTP and one for HTTPS).
 *
 * All beforeAll and afterEach hooks must be tied to the specific instance, or
 * else you get shared mutable state, and test cases interfering with each
 * other. Test isolation is especially important for this test file
 */
function scheduleTests(isHttps: boolean) {
  const mockClipboardInstance = makeMockClipboard(isHttps);

  beforeAll(() => {
    const originalNavigator = window.navigator;
    jest.spyOn(window, "navigator", "get").mockImplementation(() => ({
      ...originalNavigator,
      clipboard: mockClipboardInstance,
    }));
  });

  if (!isHttps) {
    beforeAll(() => {
      // Not the biggest fan of exposing implementation details like this, but
      // making any kind of mock for execCommand is really gnarly in general
      global.document.execCommand = jest.fn(() => {
        const dummyInput = document.querySelector("input[data-testid=dummy]");
        const inputIsFocused =
          dummyInput instanceof HTMLInputElement &&
          document.activeElement === dummyInput;

        let copySuccessful = false;
        if (inputIsFocused) {
          mockClipboardInstance.setMockText(dummyInput.value);
          copySuccessful = true;
        }

        return copySuccessful;
      });
    });
  }

  afterEach(() => {
    mockClipboardInstance.setMockText("");
  });

  const assertClipboardTextUpdate = async (
    result: ReturnType<typeof renderUseClipboard>["result"],
    textToCheck: string,
  ): Promise<void> => {
    await act(() => result.current.copyToClipboard());
    expect(result.current.showCopiedSuccess).toBe(true);

    const clipboardText = mockClipboardInstance.getMockText();
    expect(clipboardText).toEqual(textToCheck);
  };

  /**
   * Start of test cases
   */
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
