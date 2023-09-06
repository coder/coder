import { makeStyles } from "@mui/styles";
import { LogLevel } from "api/typesGenerated";
import dayjs from "dayjs";
import { FC, useMemo } from "react";
import { MONOSPACE_FONT_FAMILY } from "../../../theme/constants";
import { combineClasses } from "../../../utils/combineClasses";
import AnsiToHTML from "ansi-to-html";
import { Theme } from "@mui/material/styles";

export interface Line {
  time: string;
  output: string;
  level: LogLevel;
}

export interface LogsProps {
  lines: Line[];
  hideTimestamps?: boolean;
  lineNumbers?: boolean;
  className?: string;
}

export const Logs: FC<React.PropsWithChildren<LogsProps>> = ({
  hideTimestamps,
  lines,
  lineNumbers,
  className = "",
}) => {
  const styles = useStyles({
    lineNumbers: Boolean(lineNumbers),
  });

  return (
    <div className={combineClasses([className, styles.root])}>
      <div className={styles.scrollWrapper}>
        {lines.map((line, idx) => (
          <div className={combineClasses([styles.line, line.level])} key={idx}>
            {!hideTimestamps && (
              <>
                <span className={styles.time}>
                  {lineNumbers
                    ? idx + 1
                    : dayjs(line.time).format(`HH:mm:ss.SSS`)}
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
}> = ({ line, hideTimestamp, number, style }) => {
  const styles = useStyles({
    lineNumbers: Boolean(number),
  });
  const output = useMemo(() => {
    return convert.toHtml(line.output.split(/\r/g).pop() as string);
  }, [line.output]);

  return (
    <div className={combineClasses([styles.line, line.level])} style={style}>
      {!hideTimestamp && (
        <>
          <span className={styles.time}>
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

const useStyles = makeStyles<
  Theme,
  {
    lineNumbers: boolean;
  }
>((theme) => ({
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
    fontSize: 14,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: ({ lineNumbers }) => (lineNumbers ? logLineHeight : "auto"),
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
  space: {
    userSelect: "none",
    width: theme.spacing(3),
    display: "block",
    flexShrink: 0,
  },
  time: {
    userSelect: "none",
    width: ({ lineNumbers }) => theme.spacing(lineNumbers ? 3.5 : 12.5),
    whiteSpace: "pre",
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
}));
