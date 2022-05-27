import FormHelperText from "@material-ui/core/FormHelperText"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"
import { Stack } from "../Stack/Stack"

interface AccountFormValues {
  email: string
  username: string
}

export const Language = {
  usernameLabel: "Username",
  emailLabel: "Email",
  updateSettings: "Update settings",
}

const validationSchema = Yup.object({
  username: nameValidator(Language.usernameLabel),
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
            disabled={true}
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
            <LoadingButton loading={isLoading} type="submit" variant="contained">
              {isLoading ? "" : Language.updateSettings}
            </LoadingButton>
          </div>
        </Stack>
      </form>
    </>
  )
}
