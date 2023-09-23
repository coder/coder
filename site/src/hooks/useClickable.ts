import {
  type KeyboardEventHandler,
  type MouseEventHandler,
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
  onKeyDown: KeyboardEventHandler<T>;
}

/**
 * Exposes props to add basic click/interactive behavior to HTML elements that
 * don't traditionally have support for them.
 */
export const useClickable = <
  // T doesn't have a default type to make it more obvious that the hook expects
  // a type argument in order to work at all
  T extends ClickableElement,
>(
  // Even though onClick isn't used in any of the internal calculations, it's
  // still a required argument, just to make sure that useClickable can't
  // accidentally be called in a component without also defining click behavior
  onClick: MouseEventHandler<T>,
): UseClickableResult<T> => {
  const ref = useRef<T>(null);

  return {
    ref,
    tabIndex: 0,
    role: "button",
    onClick,

    // Most interactive elements already have this logic baked in automatically,
    // but you explicitly have to add it for non-interactive elements
    onKeyDown: (event) => {
      if (event.key === "Enter" || event.key === "Space") {
        // Can't call onClick directly because onClick needs to work with an
        // event, and mouse events + keyboard events aren't compatible; wouldn't
        // have a value to pass in. Have to use a ref to simulate a click
        ref.current?.click();
        event.stopPropagation();
      }
    },
  };
};
