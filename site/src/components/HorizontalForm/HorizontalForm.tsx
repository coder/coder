import { makeStyles } from "@material-ui/core/styles"
import {
  FormFooterProps as BaseFormFooterProps,
  FormFooter as BaseFormFooter,
} from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { FC, HTMLProps, PropsWithChildren } from "react"

export const HorizontalForm: FC<
  PropsWithChildren & HTMLProps<HTMLFormElement>
> = ({ children, ...formProps }) => {
  const styles = useStyles()

  return (
    <form {...formProps}>
      <Stack direction="column" spacing={10} className={styles.formSections}>
        {children}
      </Stack>
    </form>
  )
}

export const FormSection: FC<
  PropsWithChildren & { title: string; description: string | JSX.Element }
> = ({ children, title, description }) => {
  const styles = useStyles()

  return (
    <div className={styles.formSection}>
      <div className={styles.formSectionInfo}>
        <h2 className={styles.formSectionInfoTitle}>{title}</h2>
        <div className={styles.formSectionInfoDescription}>{description}</div>
      </div>

      {children}
    </div>
  )
}

export const FormFields: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return (
    <Stack direction="column" className={styles.formSectionFields}>
      {children}
    </Stack>
  )
}

export const FormFooter: FC<BaseFormFooterProps> = (props) => {
  const formFooterStyles = useFormFooterStyles()
  return (
    <BaseFormFooter
      {...props}
      styles={{ ...formFooterStyles, ...props.styles }}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  formSections: {
    [theme.breakpoints.down("sm")]: {
      gap: theme.spacing(8),
    },
  },

  formSection: {
    display: "flex",
    alignItems: "flex-start",
    gap: theme.spacing(15),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(2),
    },
  },

  formSectionInfo: {
    width: 312,
    flexShrink: 0,
    position: "sticky",
    top: theme.spacing(3),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      position: "initial",
    },
  },

  formSectionInfoTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
  },

  formSectionInfoDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  },

  formSectionFields: {
    width: "100%",
  },
}))

const useFormFooterStyles = makeStyles((theme) => ({
  button: {
    minWidth: theme.spacing(23),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: theme.spacing(2),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(1),
    },
  },
}))
