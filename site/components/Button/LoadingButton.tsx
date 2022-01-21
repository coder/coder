import Button, { ButtonProps } from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import * as React from "react"

export interface LoadingButtonProps extends ButtonProps {
  /** Whether or not to disable the button and show a spinner */
  loading?: boolean
}

/**
 * LoadingButton is a small wrapper around Material-UI's button to show a loading spinner
 *
 * In Material-UI 5+ - this is built-in, but since we're on an earlier version,
 * we have to roll our own.
 */
export const LoadingButton: React.FC<LoadingButtonProps> = ({ loading = false, children, ...rest }) => {
  const styles = useStyles()
  const hidden = loading ? { opacity: 0 } : undefined

  return (
    <Button {...rest} disabled={rest.disabled || loading}>
      <span style={hidden}>{children}</span>
      {loading && (
        <div className={styles.loader}>
          <CircularProgress size={18} className={styles.spinner} />
        </div>
      )}
    </Button>
  )
}

const useStyles = makeStyles((theme) => ({
  loader: {
    position: "absolute",
    top: "50%",
    left: "50%",
    transform: "translate(-50%, -50%)",
    height: 18,
    width: 18,
  },
  spinner: {
    color: theme.palette.text.disabled,
  },
}))
