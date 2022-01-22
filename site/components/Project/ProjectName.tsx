import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"

const useStyles = makeStyles((theme) => ({
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
  const styles = useStyles()

  return (
    <Typography className={styles.root} noWrap variant="body2">
      {children}
    </Typography>
  )
}
