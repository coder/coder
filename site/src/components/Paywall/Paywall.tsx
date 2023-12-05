import { type FC, type ReactNode } from "react";
import { type Interpolation, type Theme } from "@emotion/react";
import { EnterpriseBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";

export interface PaywallProps {
  children?: ReactNode;
  message: string;
  description?: string | ReactNode;
  cta?: ReactNode;
}

export const Paywall: FC<PaywallProps> = ({ message, description, cta }) => {
  return (
    <div css={styles.root}>
      <div css={{ marginBottom: 24 }}>
        <Stack direction="row" alignItems="center" justifyContent="center">
          <h5 css={styles.title}>{message}</h5>
          <EnterpriseBadge />
        </Stack>

        {description && <p css={styles.description}>{description}</p>}
      </div>
      {cta}
    </div>
  );
};

const styles = {
  root: (theme) => ({
    display: "flex",
    flexDirection: "column",
    justifyContent: "center",
    alignItems: "center",
    textAlign: "center",
    minHeight: 300,
    padding: 24,
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
  }),
  title: {
    fontWeight: 600,
    fontFamily: "inherit",
    fontSize: 24,
    margin: 0,
  },
  description: (theme) => ({
    marginTop: 16,
    fontFamily: "inherit",
    maxWidth: 420,
    lineHeight: "160%",
    color: theme.palette.text.secondary,
    fontSize: 14,
  }),
} satisfies Record<string, Interpolation<Theme>>;
