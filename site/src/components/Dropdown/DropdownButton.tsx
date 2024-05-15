import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import Button, { type ButtonProps } from "@mui/material/Button";
import type { FC } from "react";

export const DropdownButton: FC<ButtonProps> = (props) => {
  return (
    <Button {...props} endIcon={<KeyboardArrowDown role="presentation" />} />
  );
};
