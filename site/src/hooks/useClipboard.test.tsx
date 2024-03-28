import { act, renderHook } from "@testing-library/react";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";
import { ThemeProvider } from "contexts/ThemeProvider";
import {
  type UseClipboardInput,
  type UseClipboardResult,
  useClipboard,
} from "./useClipboard";

describe(useClipboard.name, () => {
  describe("HTTP (non-secure) connections", () => {
    scheduleClipboardTests({ isHttps: false });
  });

  describe("HTTPS (secure/default) connections", () => {
    scheduleClipboardTests({ isHttps: true });
  });
});

/**
 * @file This is a very weird test setup.
 *
 * There are two main things that it's fighting against to insure that the
 * clipboard functionality is working as expected:
 * 1. userEvent.setup's default global behavior
 * 2. The fact that we need to reuse the same set of test cases for two separate
 *    contexts (secure and insecure), each with their own version of global
 *    state.
 *
 * The goal of this file is to provide a shared set of test behavior that can
 * be imported into two separate test files (one for HTTP, one for HTTPS),
 * without any risk of global state conflicts.
 *
 * ---
 * For (1), normally you could call userEvent.setup to enable clipboard mocking,
 * but userEvent doesn't expose a teardown function. It also modifies the global
 * scope for the whole test file, so enabling just one userEvent session will
 * make a mock clipboard exist for all other tests, even though you didn't tell
 * them to set up a session. The mock also assumes that the clipboard API will
 * always be available, which is not true on HTTP-only connections
 *
 * Since these tests need to split hairs and differentiate between HTTP and
 * HTTPS connections, setting up a single userEvent is disastrous. It will make
 * all the tests pass, even if they shouldn't. Have to avoid that by creating a
 * custom clipboard mock.
 *
 * ---
 * For (2), we're fighting against Jest's default behavior, which is to treat
 * the test file as the main boundary for test environments, with each test case
 * able to run in parallel. That works if you have one single global state, but
 * we need two separate versions of the global state, while repeating the exact
 * same test cases for each one.
 *
 * If both tests were to be placed in the same file, Jest would not isolate them
 * and would let their setup steps interfere with each other. This leads to one
 * of two things:
 * 1. One of the global mocks overrides the other, making it so that one
 *    connection type always fails
 * 2. The two just happen not to conflict each other, through some convoluted
 *    order of operations involving closure, but you have no idea why the code
 *    is working, and it's impossible to debug.
 */
type MockClipboardEscapeHatches = Readonly<{
  getMockText: () => string;
  setMockText: (newText: string) => void;
  simulateFailure: boolean;
  setSimulateFailure: (failureMode: boolean) => void;
}>;

type MockClipboard = Readonly<Clipboard & MockClipboardEscapeHatches>;
function makeMockClipboard(isSecureContext: boolean): MockClipboard {
  let mockClipboardValue = "";
  let shouldFail = false;

  return {
    get simulateFailure() {
      return shouldFail;
    },
    setSimulateFailure: (value) => {
      shouldFail = value;
    },

    readText: async () => {
      if (shouldFail) {
        throw new Error("Clipboard deliberately failed");
      }

      if (!isSecureContext) {
        throw new Error(
          "Trying to read from clipboard outside secure context!",
        );
      }

      return mockClipboardValue;
    },
    writeText: async (newText) => {
      if (shouldFail) {
        throw new Error("Clipboard deliberately failed");
      }

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

function renderUseClipboard(inputs: UseClipboardInput) {
  return renderHook<UseClipboardResult, UseClipboardInput>(
    (props) => useClipboard(props),
    {
      initialProps: inputs,
      wrapper: ({ children }) => (
        <ThemeProvider>
          {children}
          <GlobalSnackbar />
        </ThemeProvider>
      ),
    },
  );
}

type ScheduleConfig = Readonly<{ isHttps: boolean }>;

export function scheduleClipboardTests({ isHttps }: ScheduleConfig) {
  const mockClipboardInstance = makeMockClipboard(isHttps);
  const originalNavigator = window.navigator;

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(window, "navigator", "get").mockImplementation(() => ({
      ...originalNavigator,
      clipboard: mockClipboardInstance,
    }));

    if (!isHttps) {
      // Not the biggest fan of exposing implementation details like this, but
      // making any kind of mock for execCommand is really gnarly in general
      global.document.execCommand = jest.fn(() => {
        if (mockClipboardInstance.simulateFailure) {
          return false;
        }

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
    }
  });

  afterEach(() => {
    jest.useRealTimers();
    mockClipboardInstance.setMockText("");
    mockClipboardInstance.setSimulateFailure(false);
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
    const textToCopy = "dogs";
    const { result } = renderUseClipboard({ textToCopy });
    await assertClipboardTextUpdate(result, textToCopy);
  });

  it("Should indicate to components not to show successful copy after a set period of time", async () => {
    const textToCopy = "cats";
    const { result } = renderUseClipboard({ textToCopy });
    await assertClipboardTextUpdate(result, textToCopy);

    setTimeout(() => {
      expect(result.current.showCopiedSuccess).toBe(false);
    }, 10_000);

    await jest.runAllTimersAsync();
  });

  it("Should notify the user of an error using the provided callback", async () => {
    const textToCopy = "birds";
    const onError = jest.fn();
    const { result } = renderUseClipboard({ textToCopy, onError });

    mockClipboardInstance.setSimulateFailure(true);
    await act(() => result.current.copyToClipboard());
    expect(onError).toBeCalled();
  });
}
