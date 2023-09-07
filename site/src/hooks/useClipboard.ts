import { useState } from "react";

export const useClipboard = (
  text: string,
): { isCopied: boolean; copy: () => Promise<void> } => {
  const [isCopied, setIsCopied] = useState<boolean>(false);

  const copy = async (): Promise<void> => {
    try {
      await window.navigator.clipboard.writeText(text);
      setIsCopied(true);
      window.setTimeout(() => {
        setIsCopied(false);
      }, 1000);
    } catch (err) {
      const input = document.createElement("input");
      input.value = text;
      document.body.appendChild(input);
      input.focus();
      input.select();
      const result = document.execCommand("copy");
      document.body.removeChild(input);
      if (result) {
        setIsCopied(true);
        window.setTimeout(() => {
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

  return {
    isCopied,
    copy,
  };
};
