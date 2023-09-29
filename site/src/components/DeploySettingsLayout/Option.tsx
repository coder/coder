import type { PropsWithChildren, FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import Box, { BoxProps } from "@mui/material/Box";
import { useTheme } from "@mui/system";
import { DisabledBadge, EnabledBadge } from "./Badges";
import { css } from "@emotion/react";

export const OptionName: FC<PropsWithChildren> = (props) => {
  const { children } = props;

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

export const OptionDescription: FC<PropsWithChildren> = (props) => {
  const { children } = props;
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

interface OptionValueProps {
  children?: boolean | number | string | string[];
}

export const OptionValue: FC<OptionValueProps> = (props) => {
  const { children } = props;
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

  if (!children || children.length === 0) {
    return <span css={optionStyles}>Not set</span>;
  }

  if (typeof children === "string") {
    return <span css={optionStyles}>{children}</span>;
  }

  if (Array.isArray(children)) {
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

  return <span css={optionStyles}>{JSON.stringify(children)}</span>;
};

interface OptionConfigProps extends BoxProps {
  source?: boolean;
}

// OptionalConfig takes a source bool to indicate if the Option is the source of the configured value.
export const OptionConfig = (props: OptionConfigProps) => {
  const { source, sx, ...attrs } = props;
  const theme = useTheme();
  const borderColor = source
    ? theme.palette.primary.main
    : theme.palette.divider;

  return (
    <Box
      {...attrs}
      sx={{
        fontSize: 13,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontWeight: 600,
        backgroundColor: (theme) =>
          source
            ? theme.palette.primary.dark
            : theme.palette.background.paperLight,
        display: "inline-flex",
        alignItems: "center",
        borderRadius: 0.25,
        padding: (theme) => theme.spacing(0, 1),
        border: `1px solid ${borderColor}`,
        ...sx,
      }}
    />
  );
};

interface OptionConfigFlagProps extends BoxProps {
  source?: boolean;
}

export const OptionConfigFlag = (props: OptionConfigFlagProps) => {
  const { children, source, sx, ...attrs } = props;

  return (
    <Box
      {...attrs}
      sx={{
        fontSize: 10,
        fontWeight: 600,
        margin: (theme) => theme.spacing(0, 0.75, 0, -0.5),
        display: "block",
        backgroundColor: (theme) =>
          source ? "rgba(0, 0, 0, 0.7)" : theme.palette.divider,
        lineHeight: 1,
        padding: (theme) => theme.spacing(0.25, 0.5),
        borderRadius: 0.25,
        ...sx,
      }}
    >
      {children}
    </Box>
  );
};
