import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import type { AuditDiff } from "api/typesGenerated";
import colors from "theme/tailwindColors";
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

interface AuditLogDiffProps {
  diff: AuditDiff;
}

export const AuditLogDiff: FC<AuditLogDiffProps> = ({ diff }) => {
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

  diffColumn: {
    flex: 1,
    paddingTop: 16,
    paddingBottom: 20,
    paddingRight: 16,
    lineHeight: "160%",
    alignSelf: "stretch",
    overflowWrap: "anywhere",
  },

  diffOld: {
    backgroundColor: colors.red[950],
    color: colors.red[50],
  },

  diffRow: {
    display: "flex",
    alignItems: "baseline",
  },

  diffLine: {
    opacity: 0.5,
    width: 48,
    textAlign: "right",
    flexShrink: 0,
  },

  diffIcon: (theme) => ({
    width: 32,
    textAlign: "center",
    fontSize: theme.typography.body1.fontSize,
    flexShrink: 0,
  }),

  diffNew: {
    backgroundColor: colors.green[950],
    color: colors.green[50],
  },

  diffValue: {
    padding: 1,
    borderRadius: 4,
  },

  diffValueOld: {
    backgroundColor: colors.red[800],
  },

  diffValueNew: {
    backgroundColor: colors.green[800],
  },
} satisfies Record<string, Interpolation<Theme>>;
