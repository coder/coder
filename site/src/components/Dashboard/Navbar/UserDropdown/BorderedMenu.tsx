import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import Popover, { PopoverProps } from "@mui/material/Popover";
import type { FC, PropsWithChildren } from "react";

type BorderedMenuVariant = "user-dropdown";

export type BorderedMenuProps = Omit<PopoverProps, "variant"> & {
  variant?: BorderedMenuVariant;
};

export const BorderedMenu: FC<PropsWithChildren<BorderedMenuProps>> = ({
  children,
  variant,
  ...rest
}) => {
  const theme = useTheme();

  const paper = css`
    width: 260px;
    border-radius: ${theme.shape.borderRadius};
    box-shadow: ${theme.shadows[6]};
  `;

  return (
    <Popover classes={{ paper }} data-variant={variant} {...rest}>
      {children}
    </Popover>
  );
};
