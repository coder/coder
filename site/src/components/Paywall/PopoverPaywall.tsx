import type { Interpolation, Theme } from "@emotion/react";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import type { FC, ReactNode } from "react";
import { EnterpriseBadge, PremiumBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import { docs } from "utils/docs";

export interface PopoverPaywallProps {
  message: string;
  description?: ReactNode;
  documentationLink?: string;
  licenseType?: "enterprise" | "premium";
}

export const PopoverPaywall: FC<PopoverPaywallProps> = ({
  message,
  description,
  documentationLink,
  licenseType = "enterprise",
}) => {
  return (
    <div css={styles.root}>
      <div>
        <Stack direction="row" alignItems="center" css={{ marginBottom: 18 }}>
          <h5 css={styles.title}>{message}</h5>
          {licenseType === "premium" ? <PremiumBadge /> : <EnterpriseBadge />}
        </Stack>

        {description && <p css={styles.description}>{description}</p>}
        <Link
          href={documentationLink}
          target="_blank"
          rel="noreferrer"
          css={{ fontWeight: 600 }}
        >
          Read the documentation
        </Link>
      </div>
      <div css={styles.separator}></div>
      <Stack direction="column" alignItems="center" spacing={2}>
        <ul css={styles.featureList}>
          <li css={styles.feature}>
            <FeatureIcon /> Template access control
          </li>
          <li css={styles.feature}>
            <FeatureIcon /> User groups
          </li>
          <li css={styles.feature}>
            <FeatureIcon /> 24 hour support
          </li>
          <li css={styles.feature}>
            <FeatureIcon /> Audit logs
          </li>
          {licenseType === "premium" && (
            <li css={styles.feature}>
              <FeatureIcon /> Organizations
            </li>
          )}
        </ul>
        <Button
          href={docs("/enterprise")}
          target="_blank"
          rel="noreferrer"
          startIcon={<span css={{ fontSize: 22 }}>&rarr;</span>}
          variant="outlined"
          color="neutral"
        >
          Learn about {licenseType === "premium" ? "Premium" : "Enterprise"}
        </Button>
      </Stack>
    </div>
  );
};

const FeatureIcon: FC = () => {
  return <TaskAltIcon css={styles.featureIcon} />;
};

const styles = {
  root: (theme) => ({
    display: "flex",
    flexDirection: "row",
    alignItems: "center",
    maxWidth: 600,
    padding: "24px 36px",
    backgroundImage: `linear-gradient(160deg, transparent, ${theme.roles.active.background})`,
    border: `1px solid ${theme.roles.active.fill.outline}`,
    borderRadius: 8,
    gap: 18,
  }),
  title: {
    fontWeight: 600,
    fontFamily: "inherit",
    fontSize: 18,
    margin: 0,
  },
  description: (theme) => ({
    marginTop: 8,
    fontFamily: "inherit",
    maxWidth: 420,
    lineHeight: "160%",
    color: theme.palette.text.secondary,
    fontSize: 14,
  }),
  separator: (theme) => ({
    width: 1,
    height: 180,
    backgroundColor: theme.palette.divider,
    marginLeft: 8,
  }),
  featureList: {
    listStyle: "none",
    margin: 0,
    marginRight: 8,
    padding: "0 12px",
    fontSize: 13,
    fontWeight: 500,
  },
  featureIcon: (theme) => ({
    color: theme.roles.active.fill.outline,
    fontSize: "1.5em",
  }),
  feature: {
    display: "flex",
    alignItems: "center",
    padding: 3,
    gap: 8,
    lineHeight: 1.2,
  },
} satisfies Record<string, Interpolation<Theme>>;
