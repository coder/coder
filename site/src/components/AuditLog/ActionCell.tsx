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
