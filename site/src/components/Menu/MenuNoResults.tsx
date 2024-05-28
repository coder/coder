import { useTheme } from "@emotion/react";
import type { FC } from "react";

export const MenuNoResults: FC = () => {
  const theme = useTheme();

  return (
    <div
      css={{
        padding: "8px 12px",
        color: theme.palette.text.secondary,
        fontSize: 14,
        textAlign: "center",
      }}
    >
      No results found
    </div>
  );
};
