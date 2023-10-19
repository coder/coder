import {
  ReactElement,
  ReactNode,
  cloneElement,
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
// This is used as base for the main Popover component
// eslint-disable-next-line no-restricted-imports -- Read above
import MuiPopover, {
  type PopoverProps as MuiPopoverProps,
} from "@mui/material/Popover";

type TriggerMode = "hover" | "click";

type TriggerRef = React.RefObject<HTMLElement>;

type TriggerElement = ReactElement<{
  onClick?: () => void;
  ref: TriggerRef;
}>;

type PopoverContextValue = {
  isOpen: boolean;
  setIsOpen: React.Dispatch<React.SetStateAction<boolean>>;
  triggerRef: TriggerRef;
  mode: TriggerMode;
};

const PopoverContext = createContext<PopoverContextValue | undefined>(
  undefined,
);

export const Popover = (props: {
  children: ReactNode | ((popover: PopoverContextValue) => ReactNode); // Allows inline usage
  mode?: TriggerMode;
  isDefaultOpen?: boolean;
}) => {
  const [isOpen, setIsOpen] = useState(props.isDefaultOpen ?? false);
  const triggerRef = useRef<HTMLElement>(null);
  const value = { isOpen, setIsOpen, triggerRef, mode: props.mode ?? "click" };

  return (
    <PopoverContext.Provider value={value}>
      {typeof props.children === "function"
        ? props.children(value)
        : props.children}
    </PopoverContext.Provider>
  );
};

export const usePopover = () => {
  const context = useContext(PopoverContext);
  if (!context) {
    throw new Error(
      "Popover compound components cannot be rendered outside the Popover component",
    );
  }
  return context;
};

export const PopoverTrigger = (props: {
  children: TriggerElement;
  hover?: boolean;
}) => {
  const popover = usePopover();

  const clickProps = {
    onClick: () => {
      popover.setIsOpen((isOpen) => !isOpen);
    },
  };

  const hoverProps = {
    onPointerEnter: () => {
      popover.setIsOpen(true);
    },
    onPointerLeave: () => {
      popover.setIsOpen(false);
    },
  };

  return cloneElement(props.children, {
    ...(popover.mode === "click" ? clickProps : hoverProps),
    ref: popover.triggerRef,
  });
};

type Horizontal = "left" | "right";

export const PopoverContent = (
  props: Omit<MuiPopoverProps, "open" | "onClose" | "anchorEl"> & {
    horizontal?: Horizontal;
  },
) => {
  const popover = usePopover();
  const [isReady, setIsReady] = useState(false);
  const horizontal = props.horizontal ?? "left";
  const hoverMode = popover.mode === "hover";

  // This is a hack to make sure the popover is not rendered until the trigger
  // is ready. This is a limitation on MUI that does not support defaultIsOpen
  // on Popover but we need it to storybook the component.
  useEffect(() => {
    if (!isReady && popover.triggerRef.current !== null) {
      setIsReady(true);
    }
  }, [isReady, popover.triggerRef]);

  if (!popover.triggerRef.current) {
    return null;
  }

  return (
    <MuiPopover
      disablePortal
      css={(theme) => ({
        // When it is on hover mode, and the moude is moving from the trigger to
        // the popover, if there is any space, the popover will be closed. I
        // found this is a limitation on how MUI structured the component. It is
        // not a big issue for now but we can reavaluate it in the future.
        marginTop: hoverMode ? undefined : theme.spacing(1),
        pointerEvents: hoverMode ? "none" : undefined,
        "& .MuiPaper-root": {
          minWidth: theme.spacing(40),
          fontSize: 14,
          pointerEvents: hoverMode ? "auto" : undefined,
        },
      })}
      {...horizontalProps(horizontal)}
      {...modeProps(popover)}
      {...props}
      open={popover.isOpen}
      onClose={() => popover.setIsOpen(false)}
      anchorEl={popover.triggerRef.current}
    />
  );
};

const modeProps = (popover: PopoverContextValue) => {
  if (popover.mode === "hover") {
    return {
      onMouseEnter: () => {
        popover.setIsOpen(true);
      },
      onMouseLeave: () => {
        popover.setIsOpen(false);
      },
    };
  }

  return {};
};

const horizontalProps = (horizontal: Horizontal) => {
  if (horizontal === "right") {
    return {
      anchorOrigin: {
        vertical: "bottom",
        horizontal: "right",
      },
      transformOrigin: {
        vertical: "top",
        horizontal: "right",
      },
    } as const;
  }

  return {
    anchorOrigin: {
      vertical: "bottom",
      horizontal: "left",
    },
  } as const;
};
