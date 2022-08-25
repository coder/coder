import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC } from "react"

export interface FormSectionProps {
  title: string
  description?: string
}

export const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "row",
    // Borrowed from PaperForm styles
    maxWidth: "852px",
    width: "100%",
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  descriptionContainer: {
    maxWidth: "200px",
    flex: "0 0 200px",
    display: "flex",
    flexDirection: "column",
    justifyContent: "flex-start",
    alignItems: "flex-start",
    marginTop: theme.spacing(5),
    marginBottom: theme.spacing(2),
  },
  descriptionText: {
    fontSize: "0.9em",
    lineHeight: "1em",
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(1),
  },
  contents: {
    flex: 1,
    marginTop: theme.spacing(4),
    marginBottom: theme.spacing(4),
  },
}))

export const FormSection: FC<React.PropsWithChildren<FormSectionProps>> = ({
  title,
  description,
  children,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.descriptionContainer}>
        <Typography variant="h5" color="textPrimary">
          {title}
        </Typography>
        {description && (
          <Typography className={styles.descriptionText} variant="body2" color="textSecondary">
            {description}
          </Typography>
        )}
      </div>
      <div className={styles.contents}>{children}</div>
    </div>
  )
}
