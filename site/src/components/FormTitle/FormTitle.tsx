import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC, ReactNode } from "react"

export interface FormTitleProps {
  title: string
  detail?: ReactNode
}

const useStyles = makeStyles((theme) => ({
  title: {
    marginTop: theme.spacing(6),
    marginBottom: theme.spacing(4),

    "& h3": {
      marginBottom: theme.spacing(1),
    },
  },
}))

export const FormTitle: FC<React.PropsWithChildren<FormTitleProps>> = ({ title, detail }) => {
  const styles = useStyles()

  return (
    <div className={styles.title}>
      <Typography variant="h3">{title}</Typography>
      {detail && <Typography variant="caption">{detail}</Typography>}
    </div>
  )
}
