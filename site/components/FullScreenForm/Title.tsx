import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"

export interface TitleProps {
  title: string
  organization: string
}

const useStyles = makeStyles((theme) => ({
  title: {
    textAlign: "center",
    marginBottom: theme.spacing(10),

    [theme.breakpoints.down("sm")]: {
      gridColumn: 1,
    },

    "& h3": {
      marginBottom: theme.spacing(1),
    },
  },
}))

export const Title: React.FC<TitleProps> = ({ title, organization }) => {

  const styles = useStyles()

  return <div className={styles.title} >
    <Typography variant="h3">{title}</Typography>
    <Typography variant="caption">
      In <strong>{organization}</strong> organization
    </Typography>
  </div>
}