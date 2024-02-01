import Button from "@mui/material/Button";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import type { JFrogXrayScan } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";

interface XRayScanAlertProps {
  scan: JFrogXrayScan;
}

export const XRayScanAlert: FC<XRayScanAlertProps> = ({ scan }) => {
  return (
    <div role="alert" css={styles.root}>
      <ExternalImage
        alt="JFrog logo"
        src="/icon/jfrog.svg"
        css={{ width: 40, height: 40 }}
      />
      <div>
        <span css={styles.title}>
          JFrog Xray detected new vulnerabilities for this agent
        </span>

        <ul css={styles.issues}>
          {scan.critical > 0 && (
            <li css={[styles.critical, styles.issueItem]}>
              {scan.critical} critical
            </li>
          )}
          {scan.high > 0 && (
            <li css={[styles.high, styles.issueItem]}>{scan.high} high</li>
          )}
          {scan.medium > 0 && (
            <li css={[styles.medium, styles.issueItem]}>
              {scan.medium} medium
            </li>
          )}
        </ul>
      </div>
      <div css={styles.link}>
        <Button
          component="a"
          size="small"
          variant="text"
          href={scan.results_url}
          target="_blank"
          rel="noreferrer"
        >
          Review results
        </Button>
      </div>
    </div>
  );
};

const styles = {
  root: (theme) => ({
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderLeft: 0,
    borderRight: 0,
    fontSize: 14,
    padding: "24px 16px 24px 32px",
    lineHeight: "1.5",
    display: "flex",
    alignItems: "center",
    gap: 24,
  }),
  title: {
    display: "block",
    fontWeight: 500,
  },
  issues: {
    listStyle: "none",
    margin: 0,
    padding: 0,
    fontSize: 13,
    display: "flex",
    alignItems: "center",
    gap: 16,
    marginTop: 4,
  },
  issueItem: {
    display: "flex",
    alignItems: "center",
    gap: 8,

    "&:before": {
      content: '""',
      display: "block",
      width: 6,
      height: 6,
      borderRadius: "50%",
      backgroundColor: "currentColor",
    },
  },
  critical: (theme) => ({
    color: theme.roles.error.fill.solid,
  }),
  high: (theme) => ({
    color: theme.roles.warning.fill.solid,
  }),
  medium: (theme) => ({
    color: theme.roles.notice.fill.solid,
  }),
  link: {
    marginLeft: "auto",
    alignSelf: "flex-start",
  },
} satisfies Record<string, Interpolation<Theme>>;
