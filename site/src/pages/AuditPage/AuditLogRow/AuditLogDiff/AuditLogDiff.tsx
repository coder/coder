import { makeStyles } from "@mui/styles";
import { AuditLog } from "api/typesGenerated";
import { colors } from "theme/colors";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { combineClasses } from "utils/combineClasses";
import { FC } from "react";

const getDiffValue = (value: unknown): string => {
  if (typeof value === "string") {
    return `"${value}"`;
  }

  if (Array.isArray(value)) {
    const values = value.map((v) => getDiffValue(v));
    return `[${values.join(", ")}]`;
  }

  if (value === null || value === undefined) {
    return "null";
  }

  return String(value);
};

export const AuditLogDiff: FC<{ diff: AuditLog["diff"] }> = ({ diff }) => {
  const styles = useStyles();
  const diffEntries = Object.entries(diff);

  return (
    <div className={styles.diff}>
      <div className={combineClasses([styles.diffColumn, styles.diffOld])}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} className={styles.diffRow}>
            <div className={styles.diffLine}>{index + 1}</div>
            <div className={styles.diffIcon}>-</div>
            <div>
              {attrName}:{" "}
              <span
                className={combineClasses([
                  styles.diffValue,
                  styles.diffValueOld,
                ])}
              >
                {valueDiff.secret ? "••••••••" : getDiffValue(valueDiff.old)}
              </span>
            </div>
          </div>
        ))}
      </div>
      <div className={combineClasses([styles.diffColumn, styles.diffNew])}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} className={styles.diffRow}>
            <div className={styles.diffLine}>{index + 1}</div>
            <div className={styles.diffIcon}>+</div>
            <div>
              {attrName}:{" "}
              <span
                className={combineClasses([
                  styles.diffValue,
                  styles.diffValueNew,
                ])}
              >
                {valueDiff.secret ? "••••••••" : getDiffValue(valueDiff.new)}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  diff: {
    display: "flex",
    alignItems: "flex-start",
    fontSize: theme.typography.body2.fontSize,
    borderTop: `1px solid ${theme.palette.divider}`,
    fontFamily: MONOSPACE_FONT_FAMILY,
    position: "relative",
    zIndex: 2,
  },

  diffColumn: {
    flex: 1,
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2.5),
    paddingRight: theme.spacing(2),
    lineHeight: "160%",
    alignSelf: "stretch",
    overflowWrap: "anywhere",
  },

  diffOld: {
    backgroundColor: theme.palette.error.dark,
    color: theme.palette.error.contrastText,
  },

  diffRow: {
    display: "flex",
    alignItems: "baseline",
  },

  diffLine: {
    opacity: 0.5,
    width: theme.spacing(6),
    textAlign: "right",
    flexShrink: 0,
  },

  diffIcon: {
    width: theme.spacing(4),
    textAlign: "center",
    fontSize: theme.typography.body1.fontSize,
    flexShrink: 0,
  },

  diffNew: {
    backgroundColor: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
  },

  diffValue: {
    padding: 1,
    borderRadius: theme.shape.borderRadius / 2,
  },

  diffValueOld: {
    backgroundColor: colors.red[12],
  },

  diffValueNew: {
    backgroundColor: colors.green[12],
  },
}));
