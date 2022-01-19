import React from "react"
import { Box, Typography } from "@material-ui/core"

export interface ProjectIconProps {
  title: string
  icon: string
  description?: string
  onClick: () => void
}

export const ProjectIcon: React.FC<ProjectIconProps> = ({ title, icon, onClick }) => {
  return (
    <Box
      css={{
        flex: "0",
        margin: "1em",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        border: "1px solid red",
      }}
      onClick={onClick}
    >
      <img src={icon} width={"128px"} height={"128px"} />
      <Typography>{title}</Typography>
    </Box>
  )
}
