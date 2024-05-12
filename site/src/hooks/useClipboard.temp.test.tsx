import { act, renderHook } from "@testing-library/react";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";
import { ThemeProvider } from "contexts/ThemeProvider";
import {
  type UseClipboardInput,
  type UseClipboardResult,
  useClipboard,
} from "./useClipboard";

type SetupMockClipboardResult = Readonly<{
  mockClipboard: Clipboard;
  getClipboardText: () => string;
  setClipboardText: (newText: string) => void;
  setSimulateFailure: (shouldFail: boolean) => void;
}>;

function setupMockClipboard(isSecure: boolean): SetupMockClipboardResult {
  let mockClipboardText = "";
  let shouldSimulateFailure = false;

  const mockClipboard: Clipboard = {
    readText: async () => {
      if (!isSecure) {
        throw new Error(
          "Not allowed to access clipboard outside of secure contexts",
        );
      }

      if (shouldSimulateFailure) {
        throw new Error("Failed to read from clipboard");
      }

      return mockClipboardText;
    },

    writeText: async (newText) => {
      if (!isSecure) {
        throw new Error(
          "Not allowed to access clipboard outside of secure contexts",
        );
      }

      if (shouldSimulateFailure) {
        throw new Error("Failed to write to clipboard");
      }

      mockClipboardText = newText;
    },

    // Don't need these other methods for any of the tests; read and write are
    // both synchronous and slower than the promise-based methods, so ideally
    // we won't ever need to call them in the hook logic
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
    read: jest.fn(),
    write: jest.fn(),
  };

  return {
    mockClipboard,
    getClipboardText: () => mockClipboardText,
    setClipboardText: (newText) => {
      mockClipboardText = newText;
    },
    setSimulateFailure: (newShouldFailValue) => {
      shouldSimulateFailure = newShouldFailValue;
    },
  };
}

function renderUseClipboard<TInput extends UseClipboardInput>(inputs: TInput) {
  return renderHook<UseClipboardResult, TInput>(
    (props) => useClipboard(props),
    {
      initialProps: inputs,
      wrapper: ({ children }) => (
        // Need ThemeProvider because GlobalSnackbar uses theme
        <ThemeProvider>
          {children}
          <GlobalSnackbar />
        </ThemeProvider>
      ),
    },
  );
}

const secureContextValues: readonly boolean[] = [true, false];
const originalNavigator = window.navigator;
const originalExecCommand = global.document.execCommand;

// Not a big fan of describe.each most of the time, but since we need to test
// the exact same test cases against different inputs, and we want them to run
// as sequentially as possible to minimize flakes, they make sense here
describe.each(secureContextValues)("useClipboard - secure: %j", (context) => {
  const {
    mockClipboard,
    getClipboardText,
    setClipboardText,
    setSimulateFailure,
  } = setupMockClipboard(context);

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(window, "navigator", "get").mockImplementation(() => ({
      ...originalNavigator,
      clipboard: mockClipboard,
    }));

    global.document.execCommand = jest.fn(() => {
      const dummyInput = document.querySelector("input[data-testid=dummy]");
      const inputIsFocused =
        dummyInput instanceof HTMLInputElement &&
        document.activeElement === dummyInput;

      let copySuccessful = false;
      if (inputIsFocused) {
        setClipboardText(dummyInput.value);
        copySuccessful = true;
      }

      return copySuccessful;
    });
  });

  afterEach(() => {
    jest.runAllTimers();
    jest.useRealTimers();
    jest.resetAllMocks();
    global.document.execCommand = originalExecCommand;
  });

  const assertClipboardTextUpdate = async (
    result: ReturnType<typeof renderUseClipboard>["result"],
    textToCheck: string,
  ): Promise<void> => {
    await act(() => result.current.copyToClipboard());
    expect(result.current.showCopiedSuccess).toBe(true);

    const clipboardText = getClipboardText();
    expect(clipboardText).toEqual(textToCheck);
  };

  it("Copies the current text to the user's clipboard", async () => {
    const textToCopy = "dogs";
    const { result } = renderUseClipboard({ textToCopy });
    await assertClipboardTextUpdate(result, textToCopy);
  });

  it("Should indicate to components not to show successful copy after a set period of time", async () => {
    const textToCopy = "cats";
    const { result } = renderUseClipboard({ textToCopy });
    await assertClipboardTextUpdate(result, textToCopy);

    await jest.runAllTimersAsync();
    expect(result.current.showCopiedSuccess).toBe(false);
  });

  it("Should notify the user of an error using the provided callback", async () => {
    const textToCopy = "birds";
    const onError = jest.fn();
    const { result } = renderUseClipboard({ textToCopy, onError });

    setSimulateFailure(true);
    await act(() => result.current.copyToClipboard());
    expect(onError).toBeCalled();
  });

  it.skip("Should dispatch a new toast message to the global snackbar if no callback is provided", async () => {
    expect.hasAssertions();
  });
});
