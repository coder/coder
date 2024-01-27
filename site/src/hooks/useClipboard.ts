import { useCallback, useEffect, useState } from "react";
import { useEffectEvent } from "./hookPolyfills";

type UseClipboardResult = Readonly<{
  isCopied: boolean;
  copyToClipboard: () => Promise<void>;
}>;

export const useClipboard = (textToCopy: string): UseClipboardResult => {
  // Can't initialize clipboardText with a more specific value because reading
  // is an async operation
  const [clipboardText, setClipboardText] = useState("");

  // Copy events have a ClipboardEvent associated with them, but sadly, the
  // event only gives you information about what caused the event, not the new
  // data that's just been copied. Have to use same handler for all operations
  const syncClipboardToState = useEffectEvent(async () => {
    const result = await readFromClipboard();
    setClipboardText((current) => (result.success ? result.value : current));
  });

  useEffect(() => {
    // Focus event handles case where user navigates to a different tab, copies
    // new text, and then comes back to Coder
    window.addEventListener("focus", syncClipboardToState);
    window.addEventListener("copy", syncClipboardToState);
    void syncClipboardToState();

    return () => {
      window.removeEventListener("focus", syncClipboardToState);
      window.removeEventListener("copy", syncClipboardToState);
    };
  }, [syncClipboardToState]);

  const copyToClipboard = useCallback(async () => {
    const result = await writeToClipboard(textToCopy);
    if (result.success) {
      void syncClipboardToState();
      return;
    }
  }, [syncClipboardToState, textToCopy]);

  return {
    copyToClipboard,
    isCopied: textToCopy === clipboardText,
  };
};

type VoidResult = Readonly<
  { success: true; error: null } | { success: false; error: Error }
>;

type ResultWithData<T = unknown> = Readonly<
  | { success: true; value: T; error: null }
  | { success: false; value: null; error: Error }
>;

type Result<T = unknown> = void extends T ? VoidResult : ResultWithData<T>;

async function readFromClipboard(): Promise<Result<string>> {
  // This is mainly here for the sake of being exhaustive, but the main thing it
  // helps with is suppressing error messages when Vite does HMR refreshes in
  // dev mode
  if (!document.hasFocus()) {
    return {
      success: false,
      value: null,
      error: new Error(
        "Security error - clipboard read queued while tab was not active",
      ),
    };
  }

  try {
    // navigator.clipboard is a newer API. It should be defined in most browsers
    // nowadays, but there's a fallback if not
    if (typeof window?.navigator?.clipboard?.readText === "function") {
      return {
        success: true,
        value: await window.navigator.clipboard.readText(),
        error: null,
      };
    }

    const { isExecSupported, value } = simulateClipboard("read");
    if (!isExecSupported) {
      throw new Error(
        "document.execCommand has been removed for the user's browser, but they do not have access to newer API",
      );
    }

    return {
      success: true,
      value: value,
      error: null,
    };
  } catch (err) {
    // Only expected error not covered by function logic is the user not
    // granting the webpage permission to access the clipboard
    const flattenedError =
      err instanceof Error
        ? err
        : new Error("Unknown error thrown while reading");

    return {
      success: false,
      value: null,
      error: flattenedError,
    };
  }
}

// Comments for this function mirror the ones for readFromClipboard
async function writeToClipboard(textToCopy: string): Promise<Result<void>> {
  if (!document.hasFocus()) {
    return {
      success: false,
      error: new Error(
        "Security error - clipboard read queued while tab was not active",
      ),
    };
  }

  try {
    if (typeof window?.navigator?.clipboard?.writeText === "function") {
      await window.navigator.clipboard.writeText(textToCopy);
      return { success: true, error: null };
    }

    const { isExecSupported } = simulateClipboard("write");
    if (!isExecSupported) {
      throw new Error(
        "document.execCommand has been removed for the user's browser, but they do not have access to newer API",
      );
    }

    return { success: true, error: null };
  } catch (err) {
    const flattenedError =
      err instanceof Error
        ? err
        : new Error("Unknown error thrown while reading");

    return {
      success: false,
      error: flattenedError,
    };
  }
}

type SimulateClipboardResult = Readonly<{
  isExecSupported: boolean;
  value: string;
}>;

function simulateClipboard(
  operation: "read" | "write",
): SimulateClipboardResult {
  // Absolutely cartoonish logic, but it's how you do things with the exec API
  const previousFocusTarget = document.activeElement;
  const dummyInput = document.createElement("input");
  dummyInput.style.visibility = "hidden";
  document.body.appendChild(dummyInput);
  dummyInput.focus();

  // Confusingly, you want to use the command opposite of what you actually want
  // to do to interact with the execCommand method
  const command = operation === "read" ? "paste" : "copy";
  const isExecSupported = document.execCommand(command);
  const value = dummyInput.value;
  dummyInput.remove();

  if (previousFocusTarget instanceof HTMLElement) {
    previousFocusTarget.focus();
  }

  return { isExecSupported, value };
}
