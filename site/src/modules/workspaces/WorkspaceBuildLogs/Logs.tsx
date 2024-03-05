import type { Interpolation, Theme } from "@emotion/react";
import AnsiToHTML from "ansi-to-html";
import dayjs from "dayjs";
import { type FC, type ReactNode, useMemo } from "react";
import type { LogLevel } from "api/typesGenerated";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

export const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

const convert = new AnsiToHTML();

export interface Line {
  time: string;
  output: string;
  level: LogLevel;
  source_id: string;
}

export interface LogsProps {
  lines: Line[];
  hideTimestamps?: boolean;
  className?: string;
  children?: ReactNode;
}

export const Logs: FC<LogsProps> = ({
  hideTimestamps,
  lines,
  className = "",
}) => {
  return (
    <div css={styles.root} className={`${className} logs-container`}>
      <div css={{ minWidth: "fit-content" }}>
        {lines.map((line, idx) => (
          <div
            css={[styles.line]}
            className={`${line.level} logs-line`}
            key={idx}
          >
            {!hideTimestamps && (
              <>
                <span css={styles.time}>
                  {dayjs(line.time).format(`HH:mm:ss.SSS`)}
                </span>
                <span css={styles.space} />
              </>
            )}
            <span>{line.output}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

export const logLineHeight = 20;

interface LogLineProps {
  line: Line;
  hideTimestamp?: boolean;
  number?: number;
  style?: React.CSSProperties;
  sourceIcon?: ReactNode;
  maxNumber?: number;
}

export const LogLine: FC<LogLineProps> = ({
  line,
  hideTimestamp,
  number,
  maxNumber,
  sourceIcon,
  style,
}) => {
  const output = useMemo(() => {
    return convert.toHtml(line.output.split(/\r/g).pop() as string);
  }, [line.output]);
  const isUsingLineNumber = number !== undefined;

  return (
    <div
      css={[styles.line, isUsingLineNumber && { paddingLeft: 16 }]}
      className={line.level}
      style={style}
    >
      {sourceIcon}
      {!hideTimestamp && (
        <>
          <span
            css={[styles.time, isUsingLineNumber && styles.number]}
            style={{
              minWidth: `${maxNumber ? maxNumber.toString().length - 1 : 0}em`,
            }}
          >
            {number ? number : dayjs(line.time).format(`HH:mm:ss.SSS`)}
          </span>
          <span css={styles.space} />
        </>
      )}
      <span
        dangerouslySetInnerHTML={{
          __html: output,
        }}
      />
    </div>
  );
};

const styles = {
  root: (theme) => ({
    minHeight: 156,
    padding: "8px 0",
    borderRadius: 8,
    overflowX: "auto",
    background: theme.palette.background.default,

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
      borderRadius: 0,
    },
  }),
  line: (theme) => ({
    wordBreak: "break-all",
    display: "flex",
    alignItems: "center",
    fontSize: 13,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: "auto",
    // Whitespace is significant in terminal output for alignment
    whiteSpace: "pre",
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
  space: {
    userSelect: "none",
    width: 24,
    display: "block",
    flexShrink: 0,
  },
  time: (theme) => ({
    userSelect: "none",
    whiteSpace: "pre",
    display: "inline-block",
    color: theme.palette.text.secondary,
  }),
  number: (theme) => ({
    width: 32,
    textAlign: "right",
    flexShrink: 0,
    color: theme.palette.text.disabled,
  }),
} satisfies Record<string, Interpolation<Theme>>;
