import {
  ReactElement,
  ReactNode,
  cloneElement,
  createContext,
  useContext,
  useRef,
  useState,
} from "react";
import MuiPopover, {
  type PopoverProps as MuiPopoverProps,
} from "@mui/material/Popover";

type TriggerRef = React.RefObject<HTMLElement>;

type TriggerElement = ReactElement<{
  onClick?: () => void;
  ref: TriggerRef;
}>;

type PopoverContextValue = {
  open: boolean;
  setOpen: React.Dispatch<React.SetStateAction<boolean>>;
  triggerRef: TriggerRef;
};

const PopoverContext = createContext<PopoverContextValue | undefined>(
  undefined,
);

export const Popover = (props: {
  children: ReactNode;
  defaultOpen?: boolean;
}) => {
  const [open, setOpen] = useState(props.defaultOpen ?? false);
  const triggerRef = useRef<HTMLElement>(null);

  return (
    <PopoverContext.Provider value={{ open, setOpen, triggerRef }}>
      {props.children}
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

export const PopoverTrigger = (props: { children: TriggerElement }) => {
  const popover = usePopover();

  return cloneElement(props.children, {
    onClick: () => {
      popover.setOpen((open) => !open);
    },
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
  const horizontal = props.horizontal ?? "left";

  return (
    <MuiPopover
      disablePortal
      css={(theme) => ({
        marginTop: theme.spacing(1),
        "& .MuiPaper-root": {
          width: theme.spacing(40),
        },
      })}
      {...horizontalProps(horizontal)}
      {...props}
      open={popover.open}
      onClose={() => popover.setOpen(false)}
      anchorEl={popover.triggerRef.current}
    />
  );
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
};
