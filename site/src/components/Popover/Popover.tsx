import {
  type FC,
  type ReactElement,
  type ReactNode,
  cloneElement,
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  HTMLAttributes,
} from "react";
// This is used as base for the main Popover component
// eslint-disable-next-line no-restricted-imports -- Read above
import MuiPopover, {
  type PopoverProps as MuiPopoverProps,
} from "@mui/material/Popover";

type TriggerMode = "hover" | "click";

type TriggerRef = React.RefObject<HTMLElement>;

type TriggerElement = ReactElement<{
  ref: TriggerRef;
  onClick?: () => void;
  "aria-haspopup"?: boolean;
  "aria-owns"?: string | undefined;
}>;

type PopoverContextValue = {
  id: string;
  isOpen: boolean;
  setIsOpen: React.Dispatch<React.SetStateAction<boolean>>;
  triggerRef: TriggerRef;
  mode: TriggerMode;
};

const PopoverContext = createContext<PopoverContextValue | undefined>(
  undefined,
);

export interface PopoverProps {
  children: ReactNode | ((popover: PopoverContextValue) => ReactNode); // Allows inline usage
  mode?: TriggerMode;
  isDefaultOpen?: boolean;
}

export const Popover: FC<PopoverProps> = ({
  children,
  mode,
  isDefaultOpen,
}) => {
  const hookId = useId();
  const [isOpen, setIsOpen] = useState(isDefaultOpen ?? false);
  const triggerRef = useRef<HTMLElement>(null);

  const value: PopoverContextValue = {
    isOpen,
    setIsOpen,
    triggerRef,
    id: `${hookId}-popover`,
    mode: mode ?? "click",
  };

  return (
    <PopoverContext.Provider value={value}>
      {typeof children === "function" ? children(value) : children}
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

export const PopoverTrigger = (
  props: HTMLAttributes<HTMLElement> & { children: TriggerElement },
) => {
  const popover = usePopover();
  const { children, ...elementProps } = props;

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
    ...elementProps,
    ...(popover.mode === "click" ? clickProps : hoverProps),
    "aria-haspopup": true,
    "aria-owns": popover.isOpen ? popover.id : undefined,
    ref: popover.triggerRef,
  });
};

type Horizontal = "left" | "right";

export type PopoverContentProps = Omit<
  MuiPopoverProps,
  "open" | "onClose" | "anchorEl"
> & {
  horizontal?: Horizontal;
};

export const PopoverContent: FC<PopoverContentProps> = ({
  horizontal = "left",
  ...popoverProps
}) => {
  const popover = usePopover();
  const [isReady, setIsReady] = useState(false);
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
      css={{
        // When it is on hover mode, and the mode is moving from the trigger to
        // the popover, if there is any space, the popover will be closed. I
        // found this is a limitation on how MUI structured the component. It is
        // not a big issue for now but we can re-evaluate it in the future.
        marginTop: hoverMode ? undefined : 8,
        pointerEvents: hoverMode ? "none" : undefined,
        "& .MuiPaper-root": {
          minWidth: 320,
          fontSize: 14,
          pointerEvents: hoverMode ? "auto" : undefined,
        },
      }}
      {...horizontalProps(horizontal)}
      {...modeProps(popover)}
      {...popoverProps}
      id={popover.id}
      open={popover.isOpen}
      onClose={() => popover.setIsOpen(false)}
      anchorEl={popover.triggerRef.current}
    />
  );
};

const modeProps = (popover: PopoverContextValue) => {
  if (popover.mode === "hover") {
    return {
      onPointerEnter: () => {
        popover.setIsOpen(true);
      },
      onPointerLeave: () => {
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
