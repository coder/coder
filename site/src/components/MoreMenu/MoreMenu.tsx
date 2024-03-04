import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import IconButton, { type IconButtonProps } from "@mui/material/IconButton";
import Menu, { type MenuProps } from "@mui/material/Menu";
import MenuItem, { type MenuItemProps } from "@mui/material/MenuItem";
import {
  cloneElement,
  createContext,
  type FC,
  forwardRef,
  type HTMLProps,
  type PropsWithChildren,
  type ReactElement,
  useContext,
  useRef,
  useState,
} from "react";

type MoreMenuContextValue = {
  triggerRef: React.RefObject<HTMLButtonElement>;
  close: () => void;
  open: () => void;
  isOpen: boolean;
};

const MoreMenuContext = createContext<MoreMenuContextValue | undefined>(
  undefined,
);

export const MoreMenu: FC<PropsWithChildren> = ({ children }) => {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);

  const close = () => {
    setIsOpen(false);
  };

  const open = () => {
    setIsOpen(true);
  };

  return (
    <MoreMenuContext.Provider value={{ close, open, triggerRef, isOpen }}>
      {children}
    </MoreMenuContext.Provider>
  );
};

const useMoreMenuContext = () => {
  const ctx = useContext(MoreMenuContext);

  if (!ctx) {
    throw new Error("useMoreMenuContext must be used inside of MoreMenu");
  }

  return ctx;
};

export const MoreMenuTrigger: FC<HTMLProps<HTMLButtonElement>> = ({
  children,
  ...props
}) => {
  const menu = useMoreMenuContext();

  return cloneElement(children as ReactElement, {
    "aria-haspopup": "true",
    ...props,
    ref: menu.triggerRef,
    onClick: menu.open,
  });
};

export const ThreeDotsButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  (props, ref) => {
    return (
      <IconButton
        aria-controls="more-options"
        aria-label="More options"
        ref={ref}
        {...props}
      >
        <MoreVertOutlined />
      </IconButton>
    );
  },
);

export const MoreMenuContent: FC<Omit<MenuProps, "open" | "onClose">> = (
  props,
) => {
  const menu = useMoreMenuContext();

  return (
    <Menu
      id="more-options"
      anchorEl={menu.triggerRef.current}
      open={menu.isOpen}
      onClose={menu.close}
      disablePortal
      {...props}
    />
  );
};

interface MoreMenuItemProps extends MenuItemProps {
  closeOnClick?: boolean;
  danger?: boolean;
}

export const MoreMenuItem: FC<MoreMenuItemProps> = ({
  closeOnClick = true,
  danger = false,
  ...menuItemProps
}) => {
  const ctx = useContext(MoreMenuContext);

  if (!ctx) {
    throw new Error("MoreMenuItem must be used inside of MoreMenu");
  }

  return (
    <MenuItem
      {...menuItemProps}
      css={(theme) => ({
        fontSize: 14,
        color: danger ? theme.palette.warning.light : undefined,
        "& .MuiSvgIcon-root": {
          width: 16,
          height: 16,
        },
      })}
      onClick={(e) => {
        menuItemProps.onClick && menuItemProps.onClick(e);
        if (closeOnClick) {
          ctx.close();
        }
      }}
    />
  );
};
