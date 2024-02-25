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

/**
 * May be worth adding support for the 'spinbutton' role at some point, but that
 * will change the structure of the return result in a big way. Better to wait
 * until we actually need it.
 *
 * @see {@link https://www.w3.org/WAI/ARIA/apg/patterns/spinbutton/}
 */
export type ClickableAriaRole = "button" | "switch";

export type UseClickableResult<
  TElement extends ClickableElement = ClickableElement,
  TRole extends ClickableAriaRole = ClickableAriaRole,
> = Readonly<{
  ref: RefObject<TElement>;
  tabIndex: 0;
  role: TRole;
  onClick: MouseEventHandler<TElement>;
  onKeyDown: KeyboardEventHandler<TElement>;
  onKeyUp: KeyboardEventHandler<TElement>;
}>;

/**
 * Exposes props that let you turn traditionally non-interactive elements into
 * buttons.
 */
export const useClickable = <
  TElement extends ClickableElement,
  TRole extends ClickableAriaRole = ClickableAriaRole,
>(
  onClick: MouseEventHandler<TElement>,
  role?: TRole,
): UseClickableResult<TElement, TRole> => {
  const ref = useRef<TElement>(null);

  return {
    ref,
    onClick,
    tabIndex: 0,
    role: (role ?? "button") as TRole,

    /*
     * Native buttons are programmed to handle both space and enter, but they're
     * each handled via different event handlers.
     *
     * 99% of the time, you shouldn't be able to tell the difference, but one
     * edge case behavior is that holding down Enter will continually fire
     * events, while holding down Space won't fire anything until you let go.
     */
    onKeyDown: (event) => {
      if (event.key === "Enter") {
        ref.current?.click();
        event.stopPropagation();
      }
    },
    onKeyUp: (event) => {
      if (event.key === " ") {
        ref.current?.click();
        event.stopPropagation();
      }
    },
  };
};
