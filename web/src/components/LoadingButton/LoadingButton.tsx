import Button, { ButtonProps } from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import { Theme } from "@material-ui/core/styles/createMuiTheme"
import { FC } from "react"

export interface LoadingButtonProps extends ButtonProps {
  /** Whether or not to disable the button and show a spinner */
  loading?: boolean
  /** An optional label to display with the loading spinner */
  loadingLabel?: string
}

/**
 * LoadingButton is a small wrapper around Material-UI's button to show a loading spinner
 *
 * In Material-UI 5+ - this is built-in, but since we're on an earlier version,
 * we have to roll our own.
 */
export const LoadingButton: FC<React.PropsWithChildren<LoadingButtonProps>> = ({
  loading = false,
  loadingLabel,
  children,
  ...rest
}) => {
  const styles = useStyles({ hasLoadingLabel: !!loadingLabel })
  const hidden = loading ? { opacity: 0 } : undefined

  return (
    <Button {...rest} disabled={rest.disabled || loading}>
      <span style={hidden}>{children}</span>
      {loading && (
        <div className={styles.loader}>
          <CircularProgress size={18} className={styles.spinner} />
        </div>
      )}
      {!!loadingLabel && loadingLabel}
    </Button>
  )
}

interface StyleProps {
  hasLoadingLabel?: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  loader: {
    position: (props) => {
      if (!props.hasLoadingLabel) {
        return "absolute"
      }
    },
    transform: (props) => {
      if (!props.hasLoadingLabel) {
        return "translate(-50%, -50%)"
      }
    },
    marginRight: (props) => {
      if (props.hasLoadingLabel) {
        return "10px"
      }
    },
    top: "50%",
    left: "50%",
    height: 22, // centering loading icon
    width: 18,
  },
  spinner: {
    color: theme.palette.text.disabled,
  },
}))
