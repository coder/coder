/**
 * @file Abstracts over MUI's Popover component to simplify using it (and hide
 * some of the wonkier parts of the API).
 *
 * Just place a button and some content in the component, and things just work.
 * No setup needed with hooks or refs.
 */
import {
  type KeyboardEvent,
  type MouseEvent,
  type PropsWithChildren,
  type ReactElement,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";

import { type Theme, type SystemStyleObject, Box } from "@mui/system";
import Popover, { type PopoverOrigin } from "@mui/material/Popover";
import { useNavigate, type LinkProps } from "react-router-dom";
import { useTheme } from "@emotion/react";

function getButton(container: HTMLElement) {
  return (
    container.querySelector("button") ??
    container.querySelector('[aria-role="button"]')
  );
}

const ClosePopoverContext = createContext<(() => void) | null>(null);

type PopoverLinkProps = LinkProps & {
  to: string;
  sx?: SystemStyleObject<Theme>;
};

/**
 * A custom version of a React Router Link that makes sure to close the popover
 * before starting a navigation.
 *
 * This is necessary because React Router's navigation logic doesn't work well
 * with modals (including MUI's base Popover component).
 *
 * ---
 * If the page being navigated to has lazy loading and isn't available yet, the
 * previous components are supposed to be hidden during the transition, but
 * because most React modals use React.createPortal to put content outside of
 * the main DOM tree, React Router has no way of knowing about them. So open
 * modals have a high risk of not disappearing until the page transition
 * finishes and the previous components fully unmount.
 */
export function PopoverLink({
  children,
  to,
  sx,
  ...linkProps
}: PopoverLinkProps) {
  const closePopover = useContext(ClosePopoverContext);
  if (closePopover === null) {
    throw new Error("PopoverLink is not located inside of a PopoverContainer");
  }

  // Luckily, useNavigate and Link are designed to be imperative/declarative
  // mirrors of each other, so their inputs should never get out of sync
  const navigate = useNavigate();
  const theme = useTheme();

  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    event.stopPropagation();
    closePopover();

    // Hacky, but by using a promise to push the navigation to resolve via the
    // micro-task queue, there's guaranteed to be a period for the popover to
    // close. Tried React DOM's flushSync function, but it was unreliable.
    void Promise.resolve().then(() => {
      navigate(to, linkProps);
    });
  };

  return (
    <Box
      component="a"
      // Href still needed for accessibility reasons and semantic markup
      href=""
      onClick={onClick}
      sx={{
        outline: "none",
        textDecoration: "none",
        "&:focus": {
          backgroundColor: theme.palette.action.focus,
        },
        "&:hover": {
          textDecoration: "none",
          backgroundColor: theme.palette.action.hover,
        },
        ...sx,
      }}
    >
      {children}
    </Box>
  );
}

type PopoverContainerProps = PropsWithChildren<{
  /**
   * Does not require any hooks or refs to work. Also does not override any refs
   * or event handlers attached to the button.
   */
  anchorButton: ReactElement;

  width?: number;
  originX?: PopoverOrigin["horizontal"];
  originY?: PopoverOrigin["vertical"];
  sx?: SystemStyleObject<Theme>;
}>;

export function PopoverContainer({
  children,
  anchorButton,
  originX = 0,
  originY = 0,
  width = 320,
  sx = {},
}: PopoverContainerProps) {
  const parentClosePopover = useContext(ClosePopoverContext);
  if (parentClosePopover !== null) {
    throw new Error(
      "Popover detected inside of Popover - this will always be a bad user experience",
    );
  }

  const buttonContainerRef = useRef<HTMLDivElement>(null);

  // Ref value is for effects and event listeners; state value is for React
  // renders. Have to duplicate state because after the initial render, it's
  // never safe to reference ref contents inside a render path, especially with
  // React 18 concurrency. Duplication is a necessary evil because of MUI's
  // weird, clunky APIs
  const anchorButtonRef = useRef<HTMLButtonElement | null>(null);
  const [loadedButton, setLoadedButton] = useState<HTMLButtonElement>();

  // Makes container listen to changes in button. If this approach becomes
  // untenable in the future, it can be replaced with React.cloneElement, but
  // the trade-off there is that every single anchorButton will need to be
  // wrapped inside React.forwardRef, making the abstraction leak a little more
  useEffect(() => {
    const buttonContainer = buttonContainerRef.current;
    if (buttonContainer === null) {
      throw new Error("Please attach container ref to button container");
    }

    const initialButton = getButton(buttonContainer);
    if (initialButton === null) {
      throw new Error("Initial ref query failed");
    }
    anchorButtonRef.current = initialButton;

    const onContainerMutation: MutationCallback = () => {
      const newButton = getButton(buttonContainer);
      if (newButton === null) {
        throw new Error("Semantic button removed after DOM update");
      }

      anchorButtonRef.current = newButton;
      setLoadedButton((current) => {
        return current === undefined ? undefined : newButton;
      });
    };

    const observer = new MutationObserver(onContainerMutation);
    observer.observe(buttonContainer, {
      childList: true,
      subtree: true,
    });

    return () => observer.disconnect();
  }, []);

  // Not using useInteractive because the container element is just meant to
  // catch events from the inner button, not act as a button itself
  const onInnerButtonInteraction = () => {
    if (anchorButtonRef.current === null) {
      throw new Error("Usable ref value is unavailable");
    }

    setLoadedButton(anchorButtonRef.current);
  };

  const onInnerButtonKeydown = (event: KeyboardEvent) => {
    if (event.key === "Enter" || event.key === " ") {
      onInnerButtonInteraction();
    }
  };

  const closePopover = useCallback(() => {
    setLoadedButton(undefined);
  }, []);

  return (
    <>
      {/* Cannot switch with Box component; breaks implementation */}
      <div
        // Disabling semantics for the container does not affect the button
        // placed inside; the button should still be fully accessible
        role="none"
        tabIndex={-1}
        ref={buttonContainerRef}
        onClick={onInnerButtonInteraction}
        onKeyDown={onInnerButtonKeydown}
        // Only style that container should ever need
        style={{ width: "fit-content" }}
      >
        {anchorButton}
      </div>

      <ClosePopoverContext.Provider value={closePopover}>
        <Popover
          open={loadedButton !== undefined}
          anchorEl={loadedButton}
          onClose={closePopover}
          anchorOrigin={{ horizontal: originX, vertical: originY }}
          sx={{
            "& .MuiPaper-root": {
              overflowY: "hidden",
              width,
              paddingY: 0,
              ...sx,
            },
          }}
          transitionDuration={{
            enter: 300,
            exit: 0,
          }}
        >
          {children}
        </Popover>
      </ClosePopoverContext.Provider>
    </>
  );
}
