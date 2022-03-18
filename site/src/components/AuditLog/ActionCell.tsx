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
  export const validate = (props: ActionCellProps): void => {
    if (!props.action.trim()) {
      throw new Error(`invalid action: '${props.action}'`)
    }
  }
}

export const ActionCell: React.FC<ActionCellProps> = ({ action }) => {
  ActionCellProps.validate({ action })

  return (
    <Box display="flex" alignItems="center">
      <Typography color="textSecondary" variant="caption">
        {action}
      </Typography>
    </Box>
  )
}
