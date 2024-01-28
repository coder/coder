/**
 * userEvent.setup is supposed to stub out a clipboard for you, since it doesn't
 * normally exist in JSDOM. Spent ages trying to figure out how to make it work,
 * but couldn't figure it out. So the code is using a home-grown mock.
 *
 * The bad news is that not using userEvent.setup means that you have to test
 * all events through fireEvent instead of userEvent
 *
 * @todo Figure out how to swap userEvent in, just to help make sure that the
 * tests mirror actual user flows more closely.
 */
import { fireEvent, renderHook, waitFor } from "@testing-library/react";
import { useClipboard } from "./useClipboard";

type MockClipboard = Readonly<
  Clipboard & {
    resetText: () => void;
  }
>;

function makeMockClipboard(): MockClipboard {
  let mockClipboardValue = "";

  return {
    readText: async () => mockClipboardValue,
    writeText: async (newText) => {
      mockClipboardValue = newText;
    },
    resetText: () => {
      mockClipboardValue = "";
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
  jest.spyOn(window, "navigator", "get").mockImplementation(() => {
    return { ...originalNavigator, clipboard: mockClipboard };
  });

  jest.spyOn(document, "hasFocus").mockImplementation(() => true);
  jest.useFakeTimers();
});

afterEach(() => {
  mockClipboard.resetText();
});

afterAll(() => {
  jest.resetAllMocks();
  jest.useRealTimers();
});

async function prepareInitialClipboardValue(clipboardText: string) {
  /* eslint-disable-next-line testing-library/render-result-naming-convention --
     Need to pass the whole render result back */
  const rendered = renderHook(({ text }) => useClipboard(text), {
    initialProps: { text: clipboardText },
  });

  await rendered.result.current.copyToClipboard();
  await waitFor(() => expect(rendered.result.current.isCopied).toBe(true));

  return rendered;
}

const text1 = "blah";
const text2 = "nah";

describe(useClipboard.name, () => {
  describe(".copyToClipboard", () => {
    it("Injects a new value into the clipboard when called", async () => {
      await prepareInitialClipboardValue(text1);
    });

    it("Injects the most recent value the hook rendered with", async () => {
      const { result, rerender } = await prepareInitialClipboardValue(text1);
      rerender({ text: text2 });
      await waitFor(() => expect(result.current.isCopied).toBe(false));

      await result.current.copyToClipboard();
      await waitFor(() => expect(result.current.isCopied).toBe(true));
    });

    it("Maintains a stable reference as long as text doesn't change", async () => {
      const { result, rerender } = await prepareInitialClipboardValue(text1);
      const originalFunction = result.current.copyToClipboard;

      rerender({ text: text1 });
      expect(result.current.copyToClipboard).toBe(originalFunction);

      rerender({ text: text2 });
      expect(result.current.copyToClipboard).not.toBe(originalFunction);
    });
  });

  describe(".isCopied", () => {
    it("Does not change its value if the clipboard never changes", async () => {
      const { result } = await prepareInitialClipboardValue(text1);
      for (let i = 1; i <= 10; i++) {
        setTimeout(() => {
          expect(result.current.isCopied).toBe(true);
        }, i * 10_000);
      }

      await jest.advanceTimersByTimeAsync(100_000);
    });

    it("Listens to the user copying different text while in the same tab", async () => {
      const { result } = await prepareInitialClipboardValue(text1);

      await mockClipboard.writeText(text2);
      fireEvent(window, new Event("copy"));
      await waitFor(() => expect(result.current.isCopied).toBe(false));
    });

    it("Re-syncs state when user navigates to a different tab and comes back", async () => {
      const { result, rerender } = await prepareInitialClipboardValue(text1);
      rerender({ text: text2 });
      await waitFor(() => expect(result.current.isCopied).toBe(false));

      fireEvent(window, new FocusEvent("blur"));
      await mockClipboard.writeText(text2);
      fireEvent(window, new FocusEvent("focus"));
      await waitFor(() => expect(result.current.isCopied).toBe(true));
    });
  });
});
