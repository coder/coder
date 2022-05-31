import FormHelperText from "@material-ui/core/FormHelperText"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"
import { Stack } from "../Stack/Stack"

interface SecurityFormValues {
  old_password: string
  password: string
  confirm_password: string
}

export const Language = {
  oldPasswordLabel: "Old Password",
  newPasswordLabel: "New Password",
  confirmPasswordLabel: "Confirm Password",
  oldPasswordRequired: "Old password is required",
  newPasswordRequired: "New password is required",
  confirmPasswordRequired: "Password confirmation is required",
  passwordMinLength: "Password must be at least 8 characters",
  passwordMaxLength: "Password must be no more than 64 characters",
  confirmPasswordMatch: "Password and confirmation must match",
  updatePassword: "Update password",
}

const validationSchema = Yup.object({
  old_password: Yup.string().trim().required(Language.oldPasswordRequired),
  password: Yup.string()
    .trim()
    .min(8, Language.passwordMinLength)
    .max(64, Language.passwordMaxLength)
    .required(Language.newPasswordRequired),
  confirm_password: Yup.string()
    .trim()
    .test("passwords-match", Language.confirmPasswordMatch, function (value) {
      return (this.parent as SecurityFormValues).password === value
    }),
})

export type SecurityFormErrors = FormikErrors<SecurityFormValues>
export interface SecurityFormProps {
  isLoading: boolean
  initialValues: SecurityFormValues
  onSubmit: (values: SecurityFormValues) => void
  formErrors?: SecurityFormErrors
  error?: string
}

export const SecurityForm: React.FC<SecurityFormProps> = ({
  isLoading,
  onSubmit,
  initialValues,
  formErrors = {},
  error,
}) => {
  const form: FormikContextType<SecurityFormValues> = useFormik<SecurityFormValues>({
    initialValues,
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<SecurityFormValues>(form, formErrors)

  return (
    <>
      <form onSubmit={form.handleSubmit}>
        <Stack>
          <TextField
            {...getFieldHelpers("old_password")}
            onChange={onChangeTrimmed(form)}
            autoComplete="old_password"
            fullWidth
            label={Language.oldPasswordLabel}
            variant="outlined"
            type="password"
          />
          <TextField
            {...getFieldHelpers("password")}
            onChange={onChangeTrimmed(form)}
            autoComplete="password"
            fullWidth
            label={Language.newPasswordLabel}
            variant="outlined"
            type="password"
          />
          <TextField
            {...getFieldHelpers("confirm_password")}
            onChange={onChangeTrimmed(form)}
            autoComplete="confirm_password"
            fullWidth
            label={Language.confirmPasswordLabel}
            variant="outlined"
            type="password"
          />

          {error && <FormHelperText error>{error}</FormHelperText>}

          <div>
            <LoadingButton loading={isLoading} type="submit" variant="contained">
              {isLoading ? "" : Language.updatePassword}
            </LoadingButton>
          </div>
        </Stack>
      </form>
    </>
  )
}
