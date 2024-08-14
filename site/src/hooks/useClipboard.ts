import { useEffect, useRef, useState } from "react";
import { displayError } from "components/GlobalSnackbar/utils";

const CLIPBOARD_TIMEOUT_MS = 1_000;
export const COPY_FAILED_MESSAGE = "Failed to copy text to clipboard";
export const HTTP_FALLBACK_DATA_ID = "http-fallback";

export type UseClipboardInput = Readonly<{
  textToCopy: string;

  /**
   * Optional callback to call when an error happens. If not specified, the hook
   * will dispatch an error message to the GlobalSnackbar
   */
  onError?: (errorMessage: string) => void;
}>;

export type UseClipboardResult = Readonly<{
  copyToClipboard: () => Promise<void>;
  error: Error | undefined;

  /**
   * Indicates whether the UI should show a successfully-copied status to the
   * user. When flipped to true, this will eventually flip to false, with no
   * action from the user.
   *
   * ---
   *
   * This is _not_ the same as an `isCopied` property, because the hook never
   * actually checks the clipboard to determine any state, so it is possible for
   * there to be misleading state combos like:
   * - User accidentally copies new text before showCopiedSuccess naturally
   *   flips to false
   *
   * Trying to make this property accurate enough that it could safely be called
   * `isCopied` led to browser compatibility issues in Safari.
   *
   * @see {@link https://github.com/coder/coder/pull/11863}
   */
  showCopiedSuccess: boolean;
}>;

export const useClipboard = (input: UseClipboardInput): UseClipboardResult => {
  const { textToCopy, onError: errorCallback } = input;
  const [showCopiedSuccess, setShowCopiedSuccess] = useState(false);
  const [error, setError] = useState<Error>();
  const timeoutIdRef = useRef<number | undefined>();

  useEffect(() => {
    const clearIdOnUnmount = () => window.clearTimeout(timeoutIdRef.current);
    return clearIdOnUnmount;
  }, []);

  const handleSuccessfulCopy = () => {
    setShowCopiedSuccess(true);
    timeoutIdRef.current = window.setTimeout(() => {
      setShowCopiedSuccess(false);
    }, CLIPBOARD_TIMEOUT_MS);
  };

  const copyToClipboard = async () => {
    try {
      await window.navigator.clipboard.writeText(textToCopy);
      handleSuccessfulCopy();
    } catch (err) {
      const fallbackCopySuccessful = simulateClipboardWrite(textToCopy);
      if (fallbackCopySuccessful) {
        handleSuccessfulCopy();
        return;
      }

      const wrappedErr = new Error(COPY_FAILED_MESSAGE);
      if (err instanceof Error) {
        wrappedErr.stack = err.stack;
      }

      console.error(wrappedErr);
      setError(wrappedErr);

      const notifyUser = errorCallback ?? displayError;
      notifyUser(COPY_FAILED_MESSAGE);
    }
  };

  return { showCopiedSuccess, error, copyToClipboard };
};

/**
 * Provides a fallback clipboard method for when browsers do not have access
 * to the clipboard API (the browser is older, or the deployment is only running
 * on HTTP, when the clipboard API is only available in secure contexts).
 *
 * It feels silly that you have to make a whole dummy input just to simulate a
 * clipboard, but that's really the recommended approach for older browsers.
 *
 * @see {@link https://web.dev/patterns/clipboard/copy-text?hl=en}
 */
function simulateClipboardWrite(textToCopy: string): boolean {
  const previousFocusTarget = document.activeElement;
  const dummyInput = document.createElement("input");

  // Have to add test ID to dummy element for mocking purposes in tests
  dummyInput.setAttribute("data-testid", HTTP_FALLBACK_DATA_ID);

  // Using visually-hidden styling to ensure that inserting the element doesn't
  // cause any content reflows on the page (removes any risk of UI flickers).
  // Can't use visibility:hidden or display:none, because then the elements
  // can't receive focus, which is needed for the execCommand method to work
  const style = dummyInput.style;
  style.display = "inline-block";
  style.position = "absolute";
  style.overflow = "hidden";
  style.clip = "rect(0 0 0 0)";
  style.clipPath = "rect(0 0 0 0)";
  style.height = "1px";
  style.width = "1px";
  style.margin = "-1px";
  style.padding = "0";
  style.border = "0";

  document.body.appendChild(dummyInput);
  dummyInput.value = textToCopy;
  dummyInput.focus();
  dummyInput.select();

  /**
   * The document.execCommand method is officially deprecated. Browsers are free
   * to remove the method entirely or choose to turn it into a no-op function
   * that always returns false. You cannot make any assumptions about how its
   * core functionality will be removed.
   *
   * @see {@link https://developer.mozilla.org/en-US/docs/Web/API/Clipboard}
   */
  let copySuccessful: boolean;
  try {
    copySuccessful = document?.execCommand("copy") ?? false;
  } catch {
    copySuccessful = false;
  }

  dummyInput.remove();
  if (previousFocusTarget instanceof HTMLElement) {
    previousFocusTarget.focus();
  }

  return copySuccessful;
}
