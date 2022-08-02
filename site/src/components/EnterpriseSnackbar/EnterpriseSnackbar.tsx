import IconButton from "@material-ui/core/IconButton"
import Snackbar, { SnackbarProps as MuiSnackbarProps } from "@material-ui/core/Snackbar"
import { makeStyles } from "@material-ui/core/styles"
import CloseIcon from "@material-ui/icons/Close"
import { FC } from "react"
import { combineClasses } from "../../util/combineClasses"

type EnterpriseSnackbarVariant = "error" | "info" | "success"

export interface EnterpriseSnackbarProps extends MuiSnackbarProps {
  /** Called when the snackbar should close, either from timeout or clicking close */
  onClose: () => void
  /** Variant of snackbar, for theming */
  variant?: EnterpriseSnackbarVariant
}

/**
 * Wrapper around Material UI's Snackbar component, provides pre-configured
 * themes and convenience props. Coder UI's Snackbars require a close handler,
 * since they always render a close button.
 *
 * Snackbars do _not_ automatically appear in the top-level position when
 * rendered, you'll need to use ReactDom portals or the Material UI Portal
 * component for that.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/snackbars/
 */
export const EnterpriseSnackbar: FC<React.PropsWithChildren<EnterpriseSnackbarProps>> = ({
  onClose,
  variant = "info",
  ContentProps = {},
  action,
  ...rest
}) => {
  const styles = useStyles()

  return (
    <Snackbar
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
      {...rest}
      action={
        <div className={styles.actionWrapper}>
          {action}
          <IconButton onClick={onClose} className={styles.iconButton}>
            <CloseIcon className={styles.closeIcon} />
          </IconButton>
        </div>
      }
      ContentProps={{
        ...ContentProps,
        className: combineClasses({
          [styles.snackbarContent]: true,
          [styles.snackbarContentInfo]: variant === "info",
          [styles.snackbarContentError]: variant === "error",
          [styles.snackbarContentSuccess]: variant === "success",
        }),
      }}
      onClose={onClose}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  actionWrapper: {
    display: "flex",
    alignItems: "center",
  },
  iconButton: {
    padding: 0,
  },
  closeIcon: {
    width: 25,
    height: 25,
    color: theme.palette.primary.contrastText,
  },
  snackbarContent: {
    border: `1px solid ${theme.palette.divider}`,
    borderLeft: `4px solid ${theme.palette.primary.main}`,
    borderRadius: theme.shape.borderRadius,
    padding: `
      ${theme.spacing(1)}px
      ${theme.spacing(3)}px
      ${theme.spacing(1)}px
      ${theme.spacing(2)}px
    `,
    boxShadow: theme.shadows[6],
    alignItems: "inherit",
    backgroundColor: theme.palette.background.paper,
    color: theme.palette.text.secondary,
  },
  snackbarContentInfo: {
    // Use success color as a highlight
    borderLeftColor: theme.palette.primary.main,
  },
  snackbarContentError: {
    borderLeftColor: theme.palette.error.main,
  },
  snackbarContentSuccess: {
    borderLeftColor: theme.palette.success.main,
  },
}))
