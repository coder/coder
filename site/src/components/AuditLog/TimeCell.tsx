import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import React from "react"

export interface TimeCellProps {
  date: Date
}
export namespace TimeCellProps {
  export const displayTime = (props: TimeCellProps): string => {
    return props.date.toLocaleTimeString()
  }

  export const displayDate = (props: TimeCellProps): string => {
    return props.date.toLocaleDateString().replace(/\//g, ".")
  }
}

export const TimeCell: React.FC<TimeCellProps> = (props) => {
  return (
    <Box display="flex" flexDirection="column">
      <Typography>{TimeCellProps.displayTime(props)}</Typography>
      <Typography variant="caption">{TimeCellProps.displayDate(props)}</Typography>
    </Box>
  )
}
