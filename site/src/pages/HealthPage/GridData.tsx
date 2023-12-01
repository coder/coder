import useTheme from "@mui/styles/useTheme";
import { HTMLProps } from "react";

export const GridData = (props: HTMLProps<HTMLDivElement>) => {
  return (
    <div
      css={{
        display: "grid",
        gridTemplateColumns: "auto auto",
        gap: 12,
        width: "min-content",
      }}
      {...props}
    />
  );
};

export const GridDataLabel = (props: HTMLProps<HTMLSpanElement>) => {
  const theme = useTheme();
  return (
    <span
      css={{
        fontSize: 14,
        fontWeight: 500,
        color: theme.palette.text.secondary,
      }}
      {...props}
    />
  );
};

export const GridDataValue = (props: HTMLProps<HTMLSpanElement>) => {
  const theme = useTheme();
  return (
    <span
      css={{
        fontSize: 14,
        color: theme.palette.text.primary,
      }}
      {...props}
    />
  );
};
