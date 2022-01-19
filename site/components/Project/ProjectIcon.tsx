import React from "react"
import { Box, makeStyles, SvgIcon, Typography } from "@material-ui/core"

export interface ProjectIconProps {
  title: string
  icon?: string
  description?: string
  onClick: () => void
}

const useStyles = makeStyles((theme) => ({
  container: {
    boxShadow: theme.shadows[1],
    cursor: "pointer",
    transition: "box-shadow 250ms ease-in-out",
    backgroundColor: "lightgrey",
    "&:hover": {
      boxShadow: theme.shadows[8],
    },
  },
}))

const Circle: React.FC = () => {
  return (
    <Box
      css={{
        width: "96px",
        height: "96px",
        borderRadius: "96px",
        border: "48px solid white",
      }}
    />
  )
}

const useStyles2 = makeStyles((theme) => ({
  root: {
    color: theme.palette.text.secondary,
    display: "-webkit-box", // See (1)
    marginTop: theme.spacing(0.5),
    maxWidth: "110%",
    minWidth: 0,
    overflow: "hidden", // See (1)
    textAlign: "center",
    textOverflow: "ellipsis", // See (1)
    whiteSpace: "normal", // See (1)

    // (1) - These styles, along with clamping make it so that not only
    // can text not overflow horizontally, but there can also only be a
    // maximum of 2 line breaks. This is standard behaviour on OS files
    // (ex: Windows 10 Desktop application) to prevent excessive vertical
    // line wraps. This is important in Generic Applications, as we have no
    // control over the application name used in the manifest.
    ["-webkit-line-clamp"]: 2,
    ["-webkit-box-orient"]: "vertical",
  },
}))

export const ProjectName: React.FC = ({ children }) => {
  const styles = useStyles2()

  return (
    <Typography className={styles.root} noWrap variant="body2">
      {children}
    </Typography>
  )
}

export const ProjectIcon: React.FC<ProjectIconProps> = ({ icon, title, onClick }) => {
  const styles = useStyles()

  let iconComponent

  if (typeof icon !== "undefined") {
    iconComponent = <img src={icon} width={"128px"} height={"128px"} />
  } else {
    iconComponent = (
      <Box width={"128px"} height={"128px"} style={{ display: "flex", justifyContent: "center", alignItems: "center" }}>
        <Circle />
      </Box>
    )
  }

  return (
    <Box
      css={{
        flex: "0",
        margin: "1em",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        border: "1px solid black",
        borderRadius: "4px",
      }}
      className={styles.container}
      onClick={onClick}
    >
      {iconComponent}
      <ProjectName>{title}</ProjectName>
    </Box>
  )
}
