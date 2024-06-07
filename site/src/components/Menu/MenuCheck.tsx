import CheckOutlined from "@mui/icons-material/CheckOutlined";
import type { FC } from "react";
import { MenuIcon } from "./MenuIcon";

export const MenuCheck: FC<{ isVisible: boolean }> = ({ isVisible }) => {
  return (
    <MenuIcon>
      <CheckOutlined
        role="presentation"
        css={{
          visibility: isVisible ? "visible" : "hidden",
          marginLeft: "auto",
        }}
      />
    </MenuIcon>
  );
};
