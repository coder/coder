import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC, ReactNode } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

export interface PaywallProps {
  message: string
  description?: string | React.ReactNode
  cta?: ReactNode
}

export const Paywall: FC<React.PropsWithChildren<PaywallProps>> = (props) => {
  const { message, description, cta } = props
  const styles = useStyles()

  return (
    <Box className={styles.root}>
      <div className={styles.header}>
        <Typography variant="h5" className={styles.title}>
          {message}
        </Typography>
        {description && (
          <Typography
            variant="body2"
            color="textSecondary"
            className={styles.description}
          >
            {description}
          </Typography>
        )}
      </div>
      {cta}
    </Box>
  )
}

const useStyles = makeStyles(
  (theme) => ({
    root: {
      display: "flex",
      flexDirection: "column",
      justifyContent: "center",
      alignItems: "center",
      textAlign: "center",
      minHeight: 300,
      padding: theme.spacing(3),
      fontFamily: MONOSPACE_FONT_FAMILY,
      backgroundColor: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: theme.shape.borderRadius,
    },
    header: {
      marginBottom: theme.spacing(3),
    },
    title: {
      fontWeight: 600,
      fontFamily: "inherit",
    },
    description: {
      marginTop: theme.spacing(1),
      fontFamily: "inherit",
      maxWidth: 420,
    },
  }),
  { name: "Paywall" },
)
