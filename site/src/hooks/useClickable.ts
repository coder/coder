import { KeyboardEvent } from "react";

export interface UseClickableResult {
  tabIndex: 0;
  role: "button";
  onClick: () => void;
  onKeyDown: (event: KeyboardEvent) => void;
}

export const useClickable = (onClick: () => void): UseClickableResult => {
  return {
    tabIndex: 0,
    role: "button",
    onClick,
    onKeyDown: (event: KeyboardEvent) => {
      if (event.key === "Enter") {
        onClick();
      }
    },
  };
};
