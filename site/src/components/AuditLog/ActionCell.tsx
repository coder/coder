import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import React from "react"

export interface ActionCellProps {
  action: string
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
    }
  }
}

/**
 * ActionCell is a single cell in an audit log table row that contains
 * information about an action that was taken on a resource.
 *
 * @remarks
 *
 * Some common actions are CRUD, Open, signing in etc.
 */
export const ActionCell: React.FC<ActionCellProps> = (props) => {
  const { action } = ActionCellProps.validate(props)

  return (
    <Box display="flex" alignItems="center">
      <Typography color="textSecondary" variant="caption">
        {action}
      </Typography>
    </Box>
  )
}
