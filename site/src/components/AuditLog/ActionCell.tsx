import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import makeStyles from "@material-ui/core/styles/makeStyles"
import React from "react"

export const LANGUAGE = {
  statusCodeFail: "failure",
  statusCodeSuccess: "success",
}

export interface ActionCellProps {
  action: string
  statusCode: number
}
export namespace ActionCellProps {
  /**
   * validate that the received props are valid
   *
   * @throws Error if invalid
   */
  export const validate = (props: ActionCellProps): ActionCellProps => {
    const sanitizedAction = props.action.trim()

    if (!sanitizedAction) {
      throw new Error(`invalid action: '${props.action}'`)
    }

    return {
      action: sanitizedAction,
      statusCode: props.statusCode,
    }
  }

  export const isSuccessStatus = (statusCode: ActionCellProps["statusCode"]): boolean => {
    return statusCode >= 100 && statusCode < 400
  }
}

const useStyles = makeStyles((theme) => ({
  statusText: (isSuccess: boolean) => ({
    color: isSuccess ? theme.palette.primary.main : theme.palette.error.main,
  }),
}))

/**
 * ActionCell is a single cell in an audit log table row that contains
 * information about an action that was taken on a resource.
 *
 * @remarks
 *
 * Some common actions are CRUD, Open, signing in etc.
 */
export const ActionCell: React.FC<ActionCellProps> = (props) => {
  const { action, statusCode } = ActionCellProps.validate(props)
  const isSuccess = ActionCellProps.isSuccessStatus(statusCode)
  const styles = useStyles(isSuccess)

  return (
    <Box alignItems="center" display="flex" flexDirection="column">
      <Typography color="textSecondary" variant="h6">
        {action}
      </Typography>
      <Typography className={styles.statusText} variant="caption">
        {isSuccess ? LANGUAGE.statusCodeSuccess : LANGUAGE.statusCodeFail}
      </Typography>
    </Box>
  )
}
