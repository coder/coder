import CheckOutlined from "@mui/icons-material/CheckOutlined";
import type { FC } from "react";

export const MenuCheck: FC<{ isVisible: boolean }> = ({ isVisible }) => {
  return (
    <CheckOutlined
      role="presentation"
      css={{
        width: 14,
        height: 14,
        visibility: isVisible ? "visible" : "hidden",
        marginLeft: "auto",
      }}
    />
  );
};
