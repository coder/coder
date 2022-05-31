import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC, ReactNode } from "react"

export interface FormTitleProps {
  title: string
  detail?: ReactNode
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

export const FormTitle: FC<FormTitleProps> = ({ title, detail }) => {
  const styles = useStyles()

  return (
    <div className={styles.title}>
      <Typography variant="h3">{title}</Typography>
      {detail && <Typography variant="caption">{detail}</Typography>}
    </div>
  )
}
