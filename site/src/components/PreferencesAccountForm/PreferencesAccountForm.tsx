import FormHelperText from "@material-ui/core/FormHelperText"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"
import { Stack } from "../Stack/Stack"

interface AccountFormValues {
  email: string
  username: string
}

export const Language = {
  usernameLabel: "Username",
  emailLabel: "Email",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  updatePreferences: "Update preferences",
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  username: Yup.string().trim(),
})

export type AccountFormErrors = FormikErrors<AccountFormValues>
export interface AccountFormProps {
  isLoading: boolean
  initialValues: AccountFormValues
  onSubmit: (values: AccountFormValues) => void
  formErrors?: AccountFormErrors
  error?: string
}

export const AccountForm: React.FC<AccountFormProps> = ({
  isLoading,
  onSubmit,
  initialValues,
  formErrors = {},
  error,
}) => {
  const form: FormikContextType<AccountFormValues> = useFormik<AccountFormValues>({
    initialValues,
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<AccountFormValues>(form, formErrors)

  return (
    <>
      <form onSubmit={form.handleSubmit}>
        <Stack>
          <TextField
            {...getFieldHelpers("email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
            variant="outlined"
          />
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
            fullWidth
            label={Language.usernameLabel}
            variant="outlined"
          />

          {error && <FormHelperText error>{error}</FormHelperText>}

          <div>
            <LoadingButton color="primary" loading={isLoading} type="submit" variant="contained">
              {isLoading ? "" : Language.updatePreferences}
            </LoadingButton>
          </div>
        </Stack>
      </form>
    </>
  )
}
