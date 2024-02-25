import { type UseClipboardResult, useClipboard } from "./useClipboard";
import { act, renderHook } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

beforeAll(() => {
  jest.useFakeTimers();
  userEvent.setup({
    writeToClipboard: true,
  });
});

afterAll(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
});

function renderUseClipboard(textToCopy: string) {
  type Props = Readonly<{ textToCopy: string }>;

  return renderHook<UseClipboardResult, Props>(
    ({ textToCopy }) => useClipboard(textToCopy),
    { initialProps: { textToCopy } },
  );
}

type UseClipboardTestResult = ReturnType<typeof renderUseClipboard>["result"];

// This can and should be cleaned up - trying to call the clipboard's readText
// method caused an error around blob input, even though the method takes no
// arguments whatsoever, so here's this workaround using the lower-level API
async function assertClipboardTextUpdate(
  result: UseClipboardTestResult,
  textToCheck: string,
): Promise<void> {
  await act(() => result.current.copyToClipboard());
  expect(result.current.showCopiedSuccess).toBe(true);

  const clipboardTextType = "text/plain";
  const clipboardItems = await window.navigator.clipboard.read();
  const firstItem = clipboardItems[0];

  const hasData =
    firstItem !== undefined && firstItem.types.includes(clipboardTextType);

  if (!hasData) {
    throw new Error("No clipboard items to process");
  }

  const blob = await firstItem.getType(clipboardTextType);
  const clipboardText = await blob.text();
  expect(textToCheck).toEqual(clipboardText);
}

describe(useClipboard.name, () => {
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

  it.skip("Should notify the user that a copy was not successful", () => {
    expect.hasAssertions();
  });

  it.skip("Should work on non-secure (HTTP-only) connections", async () => {
    const prevClipboard = window.navigator.clipboard;

    Object.assign(window.navigator, {
      clipboard: {
        ...prevClipboard,
        writeText: async () => {
          throw new Error(
            "Trying to call clipboard API in non-secure context!",
          );
        },
      },
    });

    const hookText = "birds";
    const { result } = renderUseClipboard(hookText);
    await assertClipboardTextUpdate(result, hookText);
  });
});
