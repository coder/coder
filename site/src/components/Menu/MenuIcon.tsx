import { css } from "@emotion/css";
import {
  type FC,
  type PropsWithChildren,
  type ReactElement,
  cloneElement,
} from "react";

type MenuIconProps = {
  size?: number;
};

export const MenuIcon: FC<PropsWithChildren<MenuIconProps>> = ({
  children,
  size = 14,
}) => {
  return cloneElement(children as ReactElement, {
    className: css({
      fontSize: size,
    }),
  });
};
