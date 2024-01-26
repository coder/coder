import { useCallback, useEffect, useState } from "react";

type UseClipboardResult = Readonly<{
  isCopied: boolean;
  copyToClipboard: () => Promise<void>;
}>;

export const useClipboard = (textToCopy: string): UseClipboardResult => {
  // Can't initialize clipboardText with a more specific value because reading
  // is an async operation
  const [clipboardText, setClipboardText] = useState("");

  useEffect(() => {
    console.log(clipboardText);
  }, [clipboardText]);

  useEffect(() => {
    // Copy events have a ClipboardEvent associated with them, but sadly, the
    // event only gives you information about what caused the event, not the new
    // data that's just been copied. Have to use same handler for all operations
    const copyClipboardToState = async () => {
      const result = await readFromClipboard();
      setClipboardText((current) => (result.success ? result.value : current));
    };

    // Focus event handles case where user navigates to a different tab, copies
    // new text, and then comes back to Coder
    window.addEventListener("focus", copyClipboardToState);
    window.addEventListener("copy", copyClipboardToState);
    void copyClipboardToState();

    return () => {
      window.removeEventListener("focus", copyClipboardToState);
      window.removeEventListener("copy", copyClipboardToState);
    };
  }, []);

  const copyToClipboard = useCallback(async () => {
    const result = await writeToClipboard(textToCopy);
    if (!result.success) {
      console.error(result.error);
    }
  }, [textToCopy]);

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

export async function readFromClipboard(): Promise<Result<string>> {
  if (!document.hasFocus()) {
    return {
      success: false,
      value: null,
      error: new Error(
        "Security issue - clipboard read queued while tab was not active",
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

export async function writeToClipboard(
  textToCopy: string,
): Promise<Result<void>> {
  return {
    success: true,
    error: null,
  };

  // // Expected throw case: user's browser is old enough that it doesn't have
  // // the navigator API
  // let wrappedError: Error | null = null;
  // try {
  //   await window.navigator.clipboard.writeText(textToCopy);
  //   return { success: true, error: null };
  // } catch (err) {
  //   wrappedError = err as Error;
  // }

  // let copySuccessful = false;
  // if (!copySuccessful) {
  //   const wrappedErr = new Error(
  //     "copyToClipboard: failed to copy text to clipboard",
  //   );

  //   if (err instanceof Error) {
  //     wrappedErr.stack = err.stack;
  //   }

  //   console.error(wrappedErr);
  // }

  // const previousFocusTarget = document.activeElement;
  // const dummyInput = document.createElement("input");
  // dummyInput.value = textToCopy;

  // document.body.appendChild(dummyInput);
  // dummyInput.focus();
  // dummyInput.select();

  // if (typeof document.execCommand !== "undefined") {
  //   copySuccessful = document.execCommand("copy");
  // }

  // if (previousFocusTarget instanceof HTMLElement) {
  //   previousFocusTarget.focus();
  // }

  // return copySuccessful;
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

  // Confusingly, you want to call the method opposite of what you want to do to
  // interact with the exec method
  const command = operation === "read" ? "paste" : "copy";
  const isExecSupported = document.execCommand(command);
  const value = dummyInput.value;
  dummyInput.remove();

  if (previousFocusTarget instanceof HTMLElement) {
    previousFocusTarget.focus();
  }

  return { isExecSupported, value };
}
