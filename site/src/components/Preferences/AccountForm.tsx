import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers, onChangeTrimmed } from "../Form"
import { LoadingButton } from "./../Button"

interface AccountFormValues {
  name: string
  email: string
  username: string
}

export const Language = {
  nameLabel: "Name",
  usernameLabel: "Username",
  emailLabel: "Email",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  updatePreferences: "Update preferences",
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  name: Yup.string().trim().optional(),
  username: Yup.string().trim(),
})

const useStyles = makeStyles((theme) => ({
  loginBtnWrapper: {
    marginTop: theme.spacing(6),
    borderTop: `1px solid ${theme.palette.action.disabled}`,
    paddingTop: theme.spacing(3),
  },
  loginTextField: {
    marginTop: theme.spacing(2),
  },
  submitBtn: {
    marginTop: theme.spacing(2),
  },
}))

export interface AccountFormProps {
  isLoading: boolean
  initialValues: AccountFormValues
  onSubmit: (values: AccountFormValues) => Promise<void>
}

export const AccountForm: React.FC<AccountFormProps> = ({ isLoading, onSubmit, initialValues }) => {
  const styles = useStyles()

  const form: FormikContextType<AccountFormValues> = useFormik<AccountFormValues>({
    initialValues,
    validationSchema,
    onSubmit,
  })

  return (
    <>
      <form onSubmit={form.handleSubmit}>
        <TextField
          {...getFormHelpers<AccountFormValues>(form, "name")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          autoComplete="name"
          className={styles.loginTextField}
          fullWidth
          label={Language.nameLabel}
          variant="outlined"
        />
        <TextField
          {...getFormHelpers<AccountFormValues>(form, "email")}
          onChange={onChangeTrimmed(form)}
          autoComplete="email"
          className={styles.loginTextField}
          fullWidth
          label={Language.emailLabel}
          variant="outlined"
        />
        <TextField
          {...getFormHelpers<AccountFormValues>(form, "username")}
          onChange={onChangeTrimmed(form)}
          autoComplete="username"
          className={styles.loginTextField}
          fullWidth
          label={Language.usernameLabel}
          variant="outlined"
        />

        <div className={styles.submitBtn}>
          <LoadingButton color="primary" loading={isLoading} type="submit" variant="contained">
            {isLoading ? "" : Language.updatePreferences}
          </LoadingButton>
        </div>
      </form>
    </>
  )
}
