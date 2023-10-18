import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Typography from "@mui/material/Typography";
import { type FC, type ReactNode } from "react";
import { type Interpolation, type Theme } from "@emotion/react";
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
          <Typography variant="h5" css={styles.title}>
            {message}
          </Typography>
          <Chip
            css={styles.enterpriseChip}
            label="Enterprise"
            size="small"
            color="primary"
          />
        </Stack>

        {description && (
          <Typography
            variant="body2"
            color="textSecondary"
            css={styles.description}
          >
            {description}
          </Typography>
        )}
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
    padding: theme.spacing(3),
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  }),
  header: (theme) => ({
    marginBottom: theme.spacing(3),
  }),
  title: {
    fontWeight: 600,
    fontFamily: "inherit",
  },
  description: (theme) => ({
    marginTop: theme.spacing(1),
    fontFamily: "inherit",
    maxWidth: 420,
    lineHeight: "160%",
  }),
  enterpriseChip: (theme) => ({
    background: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
    border: `1px solid ${theme.palette.success.light}`,
    fontSize: 13,
  }),
} satisfies Record<string, Interpolation<Theme>>;
