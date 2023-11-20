import Box from "@mui/material/Box";
import { type FC, type ReactNode } from "react";
import { type Interpolation, type Theme } from "@emotion/react";
import { EnterpriseBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";

export interface PaywallProps {
  message: string;
  description?: string | React.ReactNode;
  cta?: ReactNode;
}

export const Paywall: FC<React.PropsWithChildren<PaywallProps>> = (props) => {
  const { message, description, cta } = props;

  return (
    <Box css={styles.root}>
      <div css={styles.header}>
        <Stack direction="row" alignItems="center" justifyContent="center">
          <h5 css={styles.title}>{message}</h5>
          <EnterpriseBadge />
        </Stack>

        {description && <p css={styles.description}>{description}</p>}
      </div>
      {cta}
    </Box>
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
  header: {
    marginBottom: 24,
  },
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
  enterpriseChip: (theme) => ({
    background: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
    border: `1px solid ${theme.palette.success.light}`,
    fontSize: 13,
  }),
} satisfies Record<string, Interpolation<Theme>>;
