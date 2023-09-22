import {
  type MouseEventHandler,
  type KeyboardEvent,
  type RefObject,
  useRef,
} from "react";

// Literally any object (ideally an HTMLElement) that has a .click method
type ClickableElement = {
  click: () => void;
};

export interface UseClickableResult<
  T extends ClickableElement = ClickableElement,
> {
  ref: RefObject<T>;
  tabIndex: 0;
  role: "button";
  onClick: MouseEventHandler<T>;
  onKeyDown: (event: KeyboardEvent) => void;
}

export const useClickable = <
  // T doesn't have a default type to make it more obvious that the hook expects
  // a type argument in order to work at all
  T extends ClickableElement,
>(
  onClick: MouseEventHandler<T>,
): UseClickableResult<T> => {
  const ref = useRef<T>(null);

  return {
    ref,
    tabIndex: 0,
    role: "button",
    onClick,
    onKeyDown: (event: KeyboardEvent) => {
      if (event.key === "Enter") {
        // Can't call onClick directly because onClick needs to work with an
        // event, and mouse events + keyboard events aren't compatible; wouldn't
        // have a value to pass in. Have to use a ref to simulate a click
        ref.current?.click();
      }
    },
  };
};
