import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLAttributes } from "react";
import type { LogLevel } from "api/typesGenerated";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

export const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

export interface Line {
  time: string;
  output: string;
  level: LogLevel;
  sourceId: string;
}

type LogLineProps = {
  level: LogLevel;
} & HTMLAttributes<HTMLPreElement>;

export const LogLine: FC<LogLineProps> = ({ level, ...divProps }) => {
  return (
    <pre
      css={styles.line}
      className={`${level} ${divProps.className} logs-line`}
      {...divProps}
    />
  );
};

export const LogLinePrefix: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
  return <pre css={styles.prefix} {...props} />;
};

const styles = {
  line: (theme) => ({
    margin: 0,
    wordBreak: "break-all",
    display: "flex",
    alignItems: "center",
    fontSize: 13,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: "auto",
    padding: `0 var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,

    "&.error": {
      backgroundColor: theme.roles.error.background,
      color: theme.roles.error.text,

      "& .dashed-line": {
        backgroundColor: theme.roles.error.outline,
      },
    },

    "&.debug": {
      backgroundColor: theme.roles.info.background,
      color: theme.roles.info.text,

      "& .dashed-line": {
        backgroundColor: theme.roles.info.outline,
      },
    },

    "&.warn": {
      backgroundColor: theme.roles.warning.background,
      color: theme.roles.warning.text,

      "& .dashed-line": {
        backgroundColor: theme.roles.warning.outline,
      },
    },
  }),

  prefix: (theme) => ({
    userSelect: "none",
    margin: 0,
    display: "inline-block",
    color: theme.palette.text.secondary,
    marginRight: 24,
  }),
} satisfies Record<string, Interpolation<Theme>>;
