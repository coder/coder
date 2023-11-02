import { useRef, useState, createContext, useContext } from "react";
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import Menu, { MenuProps } from "@mui/material/Menu";
import MenuItem, { MenuItemProps } from "@mui/material/MenuItem";
import IconButton from "@mui/material/IconButton";

const MoreMenuContext = createContext<{ close: () => void } | undefined>(
  undefined,
);

export const MoreMenu = (props: Omit<MenuProps, "open" | "onClose">) => {
  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const { id = "more-options" } = props;

  const close = () => {
    setIsMenuOpen(false);
  };

  return (
    <MoreMenuContext.Provider value={{ close }}>
      <IconButton
        aria-controls={id}
        aria-haspopup="true"
        onClick={() => setIsMenuOpen(true)}
        ref={menuTriggerRef}
        arial-label="More options"
      >
        <MoreVertOutlined />
      </IconButton>

      <Menu
        {...props}
        id={id}
        anchorEl={menuTriggerRef.current}
        open={isMenuOpen}
        onClose={close}
        disablePortal
      />
    </MoreMenuContext.Provider>
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
