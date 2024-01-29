import { useEffect, useRef, useState } from "react";

type UseClipboardResult = Readonly<{
  isCopied: boolean;
  copyToClipboard: () => Promise<void>;
}>;

export const useClipboard = (textToCopy: string): UseClipboardResult => {
  const [isCopied, setIsCopied] = useState(false);
  const timeoutIdRef = useRef<number | undefined>();

  useEffect(() => {
    const clearIdsOnUnmount = () => window.clearTimeout(timeoutIdRef.current);
    return clearIdsOnUnmount;
  }, []);

  const copyToClipboard = async () => {
    try {
      await window.navigator.clipboard.writeText(textToCopy);
      setIsCopied(true);
      timeoutIdRef.current = window.setTimeout(() => {
        setIsCopied(false);
      }, 1000);
    } catch (err) {
      const isExecSupported = simulateClipboardWrite();
      if (isExecSupported) {
        setIsCopied(true);
        timeoutIdRef.current = window.setTimeout(() => {
          setIsCopied(false);
        }, 1000);
      } else {
        const wrappedErr = new Error(
          "copyToClipboard: failed to copy text to clipboard",
        );
        if (err instanceof Error) {
          wrappedErr.stack = err.stack;
        }
        console.error(wrappedErr);
      }
    }
  };

  return { isCopied: isCopied, copyToClipboard };
};

function simulateClipboardWrite(): boolean {
  const previousFocusTarget = document.activeElement;
  const dummyInput = document.createElement("input");
  dummyInput.style.visibility = "hidden";
  document.body.appendChild(dummyInput);
  dummyInput.focus();
  dummyInput.select();

  const isExecSupported = document.execCommand("copy");
  dummyInput.remove();

  if (previousFocusTarget instanceof HTMLElement) {
    previousFocusTarget.focus();
  }

  return isExecSupported;
}
