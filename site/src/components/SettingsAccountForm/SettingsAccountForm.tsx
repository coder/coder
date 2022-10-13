import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import * as Yup from "yup"
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"
import { Stack } from "../Stack/Stack"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

export interface AccountFormValues {
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

export interface AccountFormProps {
  editable: boolean
  email: string
  isLoading: boolean
  initialValues: AccountFormValues
  onSubmit: (values: AccountFormValues) => void
  updateProfileError?: Error | unknown
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<AccountFormValues>
}

export const AccountForm: FC<React.PropsWithChildren<AccountFormProps>> = ({
  editable,
  email,
  isLoading,
  onSubmit,
  initialValues,
  updateProfileError,
  initialTouched,
}) => {
  const form: FormikContextType<AccountFormValues> =
    useFormik<AccountFormValues>({
      initialValues,
      validationSchema,
      onSubmit,
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<AccountFormValues>(
    form,
    updateProfileError,
  )

  return (
    <>
      <form onSubmit={form.handleSubmit}>
        <Stack>
          {Boolean(updateProfileError) && (
            <AlertBanner severity="error" error={updateProfileError} />
          )}
          <TextField
            disabled
            fullWidth
            label={Language.emailLabel}
            value={email}
            variant="outlined"
          />
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            aria-disabled={!editable}
            autoComplete="username"
            disabled={!editable}
            fullWidth
            label={Language.usernameLabel}
            variant="outlined"
          />

          <div>
            <LoadingButton
              loading={isLoading}
              aria-disabled={!editable}
              disabled={!editable}
              type="submit"
              variant="contained"
            >
              {isLoading ? "" : Language.updateSettings}
            </LoadingButton>
          </div>
        </Stack>
      </form>
    </>
  )
}
