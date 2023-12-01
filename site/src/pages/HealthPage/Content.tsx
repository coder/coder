/* eslint-disable jsx-a11y/heading-has-content -- infer from props */
import useTheme from "@mui/styles/useTheme";
import { HTMLProps } from "react";

const SIDE_PADDING = 36;

export const Header = (props: HTMLProps<HTMLDivElement>) => {
  return (
    <header
      css={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: `28px ${SIDE_PADDING}px`,
      }}
      {...props}
    />
  );
};

export const HeaderTitle = (props: HTMLProps<HTMLHeadingElement>) => {
  return (
    <h2
      css={{ margin: 0, lineHeight: "120%", fontSize: 20, fontWeight: 500 }}
      {...props}
    />
  );
};

export const Main = (props: HTMLProps<HTMLDivElement>) => {
  return <main css={{ padding: `0 ${SIDE_PADDING}px` }} {...props} />;
};

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
