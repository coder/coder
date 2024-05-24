import Button, { type ButtonProps } from "@mui/material/Button";
import { forwardRef } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";

export const MenuButton = forwardRef<HTMLButtonElement, ButtonProps>(
  (props, ref) => {
    return <Button endIcon={<DropdownArrow />} ref={ref} {...props} />;
  },
);
