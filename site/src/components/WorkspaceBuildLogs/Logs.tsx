import { makeStyles } from "@mui/styles";
import { LogLevel } from "api/typesGenerated";
import dayjs from "dayjs";
import { FC, useMemo } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { combineClasses } from "utils/combineClasses";
import AnsiToHTML from "ansi-to-html";

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
  const styles = useStyles();

  return (
    <div className={combineClasses([className, styles.root])}>
      <div className={styles.scrollWrapper}>
        {lines.map((line, idx) => (
          <div className={combineClasses([styles.line, line.level])} key={idx}>
            {!hideTimestamps && (
              <>
                <span className={styles.time}>
                  {dayjs(line.time).format(`HH:mm:ss.SSS`)}
                </span>
                <span className={styles.space} />
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
  sourceIcon?: JSX.Element;
  maxNumber?: number;
}> = ({ line, hideTimestamp, number, maxNumber, sourceIcon, style }) => {
  const styles = useStyles();
  const output = useMemo(() => {
    return convert.toHtml(line.output.split(/\r/g).pop() as string);
  }, [line.output]);
  const isUsingLineNumber = number !== undefined;

  return (
    <div
      className={combineClasses([
        styles.line,
        line.level,
        isUsingLineNumber && styles.lineNumber,
      ])}
      style={style}
    >
      {sourceIcon}
      {!hideTimestamp && (
        <>
          <span
            className={combineClasses([
              styles.time,
              isUsingLineNumber && styles.number,
            ])}
            style={{
              minWidth: `${maxNumber ? maxNumber.toString().length - 1 : 0}em`,
            }}
          >
            {number ? number : dayjs(line.time).format(`HH:mm:ss.SSS`)}
          </span>
          <span className={styles.space} />
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

const useStyles = makeStyles((theme) => ({
  root: {
    minHeight: 156,
    padding: theme.spacing(1, 0),
    borderRadius: theme.shape.borderRadius,
    overflowX: "auto",
    background: theme.palette.background.default,

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
      borderRadius: 0,
    },
  },
  scrollWrapper: {
    minWidth: "fit-content",
  },
  line: {
    wordBreak: "break-all",
    display: "flex",
    alignItems: "center",
    fontSize: 14,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: "auto",
    // Whitespace is significant in terminal output for alignment
    whiteSpace: "pre",
    padding: theme.spacing(0, 4),

    "&.error": {
      backgroundColor: theme.palette.error.dark,
    },

    "&.debug": {
      backgroundColor: theme.palette.background.paperLight,
    },

    "&.warn": {
      backgroundColor: theme.palette.warning.dark,
    },
  },
  lineNumber: {
    paddingLeft: theme.spacing(2),
  },
  space: {
    userSelect: "none",
    width: theme.spacing(3),
    display: "block",
    flexShrink: 0,
  },
  time: {
    userSelect: "none",
    whiteSpace: "pre",
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
  number: {
    width: theme.spacing(4),
    textAlign: "right",
  },
}));
