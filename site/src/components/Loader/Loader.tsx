import CircularProgress from "@mui/material/CircularProgress";
import { type FC, type HTMLAttributes } from "react";

interface LoaderProps extends HTMLAttributes<HTMLDivElement> {
  size?: number;
}

export const Loader: FC<LoaderProps> = ({ size = 26, ...attrs }) => {
  return (
    <div
      css={{
        padding: 32,
        width: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
      data-testid="loader"
      data-chromatic="ignore"
      {...attrs}
    >
      <CircularProgress size={size} />
    </div>
  );
};
