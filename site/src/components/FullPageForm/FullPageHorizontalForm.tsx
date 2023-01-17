import { Margins } from "components/Margins/Margins"
import { FC, ReactNode } from "react"
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader"
import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"

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
    <Margins size="medium">
      <PageHeader
        actions={
          <Button size="small" onClick={onCancel}>
            Cancel
          </Button>
        }
      >
        <PageHeaderTitle>{title}</PageHeaderTitle>
        {detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
      </PageHeader>

      <main className={styles.form}>{children}</main>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  form: {
    marginTop: theme.spacing(1),
  },
}))
