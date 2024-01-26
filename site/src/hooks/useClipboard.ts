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

type Result<T = unknown> = Readonly<
  | {
      success: false;
      value: null;
      error: Error;
    }
  | (void extends T
      ? { success: true; error: null }
      : { success: true; value: T; error: null })
>;

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

    // Yes, this is the "proper" way to do things with the old API - very hokey
    const previousFocusTarget = document.activeElement;
    const dummyInput = document.createElement("input");
    dummyInput.setAttribute("hidden", "true");
    document.body.appendChild(dummyInput);
    dummyInput.focus();

    const success = document.execCommand("paste");
    if (!success) {
      throw new Error("Failed to simulate clipboard with ");
    }

    const newText = dummyInput.value;
    document.body.removeChild(dummyInput);

    if (previousFocusTarget instanceof HTMLElement) {
      previousFocusTarget.focus();
    }

    return {
      success: true,
      value: newText,
      error: null,
    };
  } catch (err) {
    // Only expected error at this point is the user not granting the webpage
    // permission to access the clipboard
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
