import Popover, { PopoverProps } from "@mui/material/Popover";
import { makeStyles } from "@mui/styles";
import { FC, PropsWithChildren } from "react";

type BorderedMenuVariant = "user-dropdown";

export type BorderedMenuProps = Omit<PopoverProps, "variant"> & {
  variant?: BorderedMenuVariant;
};

export const BorderedMenu: FC<PropsWithChildren<BorderedMenuProps>> = ({
  children,
  variant,
  ...rest
}) => {
  const styles = useStyles();

  return (
    <Popover
      classes={{ paper: styles.paperRoot }}
      data-variant={variant}
      {...rest}
    >
      {children}
    </Popover>
  );
};

const useStyles = makeStyles((theme) => ({
  paperRoot: {
    width: 260,
    borderRadius: theme.shape.borderRadius,
    boxShadow: theme.shadows[6],
  },
}));
