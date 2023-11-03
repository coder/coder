import { useRef, useState, createContext, useContext, ReactNode } from "react";
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import Menu, { MenuProps } from "@mui/material/Menu";
import MenuItem, { MenuItemProps } from "@mui/material/MenuItem";
import IconButton, { IconButtonProps } from "@mui/material/IconButton";

type MoreMenuContextValue = {
  triggerRef: React.RefObject<HTMLButtonElement>;
  close: () => void;
  open: () => void;
  isOpen: boolean;
};

const MoreMenuContext = createContext<MoreMenuContextValue | undefined>(
  undefined,
);

export const MoreMenu = (props: { children: ReactNode }) => {
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
      {props.children}
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

export const MoreMenuTrigger = (props: IconButtonProps) => {
  const menu = useMoreMenuContext();

  return (
    <IconButton
      aria-controls="more-options"
      aria-label="More options"
      aria-haspopup="true"
      onClick={menu.open}
      ref={menu.triggerRef}
      {...props}
    >
      <MoreVertOutlined />
    </IconButton>
  );
};

export const MoreMenuContent = (props: Omit<MenuProps, "open" | "onClose">) => {
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

export const MoreMenuItem = (
  props: MenuItemProps & { closeOnClick?: boolean; danger?: boolean },
) => {
  const { closeOnClick = true, danger = false, ...menuItemProps } = props;
  const ctx = useContext(MoreMenuContext);

  if (!ctx) {
    throw new Error("MoreMenuItem must be used inside of MoreMenu");
  }

  return (
    <MenuItem
      {...menuItemProps}
      css={(theme) => ({
        fontSize: 14,
        color: danger ? theme.palette.error.light : undefined,
        "& .MuiSvgIcon-root": {
          width: theme.spacing(2),
          height: theme.spacing(2),
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
