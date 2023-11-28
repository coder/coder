import { type Interpolation, type Theme } from "@emotion/react";
import type { LogLevel } from "api/typesGenerated";
import dayjs from "dayjs";
import { type FC, type ReactNode, useMemo } from "react";
import AnsiToHTML from "ansi-to-html";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

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
}

export const Logs: FC<React.PropsWithChildren<LogsProps>> = ({
  hideTimestamps,
  lines,
  className = "",
}) => {
  return (
    <div css={styles.root} className={className}>
      <div css={{ minWidth: "fit-content" }}>
        {lines.map((line, idx) => (
          <div css={styles.line} className={line.level} key={idx}>
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

const convert = new AnsiToHTML();

export const LogLine: FC<{
  line: Line;
  hideTimestamp?: boolean;
  number?: number;
  style?: React.CSSProperties;
  sourceIcon?: ReactNode;
  maxNumber?: number;
}> = ({ line, hideTimestamp, number, maxNumber, sourceIcon, style }) => {
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
    fontSize: 14,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: "auto",
    // Whitespace is significant in terminal output for alignment
    whiteSpace: "pre",
    padding: "0 32px",

    "&.error": {
      backgroundColor: theme.palette.error.dark,
    },

    "&.debug": {
      backgroundColor: theme.palette.background.paper,
    },

    "&.warn": {
      backgroundColor: theme.palette.warning.dark,
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
  number: {
    width: 32,
    textAlign: "right",
  },
} satisfies Record<string, Interpolation<Theme>>;
