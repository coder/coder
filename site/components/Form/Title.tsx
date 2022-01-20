import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"

export interface TitleProps {
  title: string
  detail: React.ReactNode
}

const useStyles = makeStyles((theme) => ({
  title: {
    textAlign: "center",
    marginTop: theme.spacing(5),
    marginBottom: theme.spacing(5),

    "& h3": {
      marginBottom: theme.spacing(1),
    },
  },
}))

export const Title: React.FC<TitleProps> = ({ title, detail }) => {
  const styles = useStyles()

  return (
    <div className={styles.title}>
      <Typography variant="h3">{title}</Typography>
      <Typography variant="caption">{detail}</Typography>
    </div>
  )
}
