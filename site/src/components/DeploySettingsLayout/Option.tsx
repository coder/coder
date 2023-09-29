import type { PropsWithChildren, FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import Box, { BoxProps } from "@mui/material/Box";
import { useTheme } from "@mui/system";
import { DisabledBadge, EnabledBadge } from "./Badges";
import { css } from "@emotion/react";

export const OptionName: FC<PropsWithChildren> = ({ children }) => {
  return (
    <span
      css={{
        display: "block",
      }}
    >
      {children}
    </span>
  );
};

export const OptionDescription: FC<PropsWithChildren> = ({ children }) => {
  const theme = useTheme();
  return (
    <span
      css={{
        display: "block",
        color: theme.palette.text.secondary,
        fontSize: 14,
        marginTop: theme.spacing(0.5),
      }}
    >
      {children}
    </span>
  );
};

export const OptionValue: FC<{ children?: unknown }> = ({ children }) => {
  const theme = useTheme();

  const optionStyles = css`
    font-size: 14px;
    font-family: ${MONOSPACE_FONT_FAMILY};
    overflow-wrap: anywhere;
    user-select: all;

    & ul {
      padding: ${theme.spacing(2)};
    }
  `;

  if (typeof children === "boolean") {
    return children ? <EnabledBadge /> : <DisabledBadge />;
  }

  if (typeof children === "number") {
    return <span css={optionStyles}>{children}</span>;
  }

  if (typeof children === "string") {
    return <span css={optionStyles}>{children}</span>;
  }

  if (Array.isArray(children)) {
    if (children.length === 0) {
      return <span css={optionStyles}>Not set</span>;
    }

    return (
      <ul
        css={{
          margin: 0,
          padding: 0,
          listStylePosition: "inside",
          display: "flex",
          flexDirection: "column",
          gap: theme.spacing(0.5),
        }}
      >
        {children.map((item) => (
          <li key={item} css={optionStyles}>
            {item}
          </li>
        ))}
      </ul>
    );
  }

  if (children === "") {
    return <span css={optionStyles}>Not set</span>;
  }

  return <span css={optionStyles}>{JSON.stringify(children)}</span>;
};

export const OptionConfig = (props: BoxProps) => {
  return (
    <Box
      {...props}
      sx={{
        fontSize: 13,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontWeight: 600,
        backgroundColor: (theme) => theme.palette.background.paperLight,
        display: "inline-flex",
        alignItems: "center",
        borderRadius: 0.25,
        padding: (theme) => theme.spacing(0, 1),
        border: (theme) => `1px solid ${theme.palette.divider}`,
        ...props.sx,
      }}
    />
  );
};

export const OptionConfigFlag = (props: BoxProps) => {
  return (
    <Box
      {...props}
      sx={{
        fontSize: 10,
        fontWeight: 600,
        margin: (theme) => theme.spacing(0, 0.75, 0, -0.5),
        display: "block",
        backgroundColor: (theme) => theme.palette.divider,
        lineHeight: 1,
        padding: (theme) => theme.spacing(0.25, 0.5),
        borderRadius: 0.25,
        ...props.sx,
      }}
    />
  );
};
