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
      const input = document.createElement("input");
      input.value = textToCopy;
      document.body.appendChild(input);
      input.focus();
      input.select();
      const isCopied = document.execCommand("copy");
      document.body.removeChild(input);

      if (isCopied) {
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

  return { isCopied, copyToClipboard };
};
