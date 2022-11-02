import { FormCloseButton } from "../FormCloseButton/FormCloseButton"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { Margins } from "components/Margins/Margins"
import { FC, ReactNode } from "react"

export interface FormTitleProps {
  title: string
  detail?: ReactNode
}

export interface FullPageHorizontalFormProps {
  title: string
  detail?: ReactNode
  onCancel: () => void
}

export const FullPageHorizontalForm: FC<
  React.PropsWithChildren<FullPageHorizontalFormProps>
> = ({ title, detail, onCancel, children }) => {
  const styles = useStyles()

  return (
    <>
      <header className={styles.title}>
        <Margins size="medium">
          <Typography variant="h3">{title}</Typography>
          {detail && <Typography variant="caption">{detail}</Typography>}
        </Margins>
      </header>

      <FormCloseButton onClose={onCancel} />

      <main className={styles.main}>
        <Margins size="medium">{children}</Margins>
      </main>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    paddingTop: theme.spacing(6),
    paddingBottom: theme.spacing(8),

    [theme.breakpoints.down("sm")]: {
      paddingTop: theme.spacing(4),
      paddingBottom: theme.spacing(4),
    },
  },

  main: {
    paddingBottom: theme.spacing(10),
  },
}))
