import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import type { AuditLog } from "api/typesGenerated";
import { colors } from "theme/colors";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

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
  const diffEntries = Object.entries(diff);

  return (
    <div css={styles.diff}>
      <div css={[styles.diffColumn, styles.diffOld]}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} css={styles.diffRow}>
            <div css={styles.diffLine}>{index + 1}</div>
            <div css={styles.diffIcon}>-</div>
            <div>
              {attrName}:{" "}
              <span css={[styles.diffValue, styles.diffValueOld]}>
                {valueDiff.secret ? "••••••••" : getDiffValue(valueDiff.old)}
              </span>
            </div>
          </div>
        ))}
      </div>
      <div css={[styles.diffColumn, styles.diffNew]}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} css={styles.diffRow}>
            <div css={styles.diffLine}>{index + 1}</div>
            <div css={styles.diffIcon}>+</div>
            <div>
              {attrName}:{" "}
              <span css={[styles.diffValue, styles.diffValueNew]}>
                {valueDiff.secret ? "••••••••" : getDiffValue(valueDiff.new)}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

const styles = {
  diff: (theme) => ({
    display: "flex",
    alignItems: "flex-start",
    fontSize: theme.typography.body2.fontSize,
    borderTop: `1px solid ${theme.palette.divider}`,
    fontFamily: MONOSPACE_FONT_FAMILY,
    position: "relative",
    zIndex: 2,
  }),

  diffColumn: (theme) => ({
    flex: 1,
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2.5),
    paddingRight: theme.spacing(2),
    lineHeight: "160%",
    alignSelf: "stretch",
    overflowWrap: "anywhere",
  }),

  diffOld: (theme) => ({
    backgroundColor: theme.palette.error.dark,
    color: theme.palette.error.contrastText,
  }),

  diffRow: {
    display: "flex",
    alignItems: "baseline",
  },

  diffLine: (theme) => ({
    opacity: 0.5,
    width: theme.spacing(6),
    textAlign: "right",
    flexShrink: 0,
  }),

  diffIcon: (theme) => ({
    width: theme.spacing(4),
    textAlign: "center",
    fontSize: theme.typography.body1.fontSize,
    flexShrink: 0,
  }),

  diffNew: (theme) => ({
    backgroundColor: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
  }),

  diffValue: (theme) => ({
    padding: 1,
    borderRadius: theme.shape.borderRadius / 2,
  }),

  diffValueOld: {
    backgroundColor: colors.red[12],
  },

  diffValueNew: {
    backgroundColor: colors.green[12],
  },
} satisfies Record<string, Interpolation<Theme>>;
