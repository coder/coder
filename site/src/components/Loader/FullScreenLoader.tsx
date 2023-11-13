import CircularProgress from "@mui/material/CircularProgress";
import type { FC } from "react";

export const FullScreenLoader: FC = () => {
  return (
    <div
      css={(theme) => ({
        position: "absolute",
        top: "0",
        left: "0",
        right: "0",
        bottom: "0",
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
        background: theme.palette.background.default,
      })}
      data-testid="loader"
    >
      <CircularProgress />
    </div>
  );
};
