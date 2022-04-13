import TextField from "@material-ui/core/TextField"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers, onChangeTrimmed } from "../Form"
import { FormStack } from "../Form/FormStack"
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
  name: Yup.string().optional(),
  username: Yup.string().trim(),
})

export interface AccountFormProps {
  isLoading: boolean
  initialValues: AccountFormValues
  onSubmit: (values: AccountFormValues) => Promise<void>
}

export const AccountForm: React.FC<AccountFormProps> = ({ isLoading, onSubmit, initialValues }) => {
  const form: FormikContextType<AccountFormValues> = useFormik<AccountFormValues>({
    initialValues,
    validationSchema,
    onSubmit,
  })

  return (
    <>
      <form onSubmit={form.handleSubmit}>
        <FormStack>
          <TextField
            {...getFormHelpers<AccountFormValues>(form, "name")}
            autoFocus
            autoComplete="name"
            fullWidth
            label={Language.nameLabel}
            variant="outlined"
          />
          <TextField
            {...getFormHelpers<AccountFormValues>(form, "email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
            variant="outlined"
          />
          <TextField
            {...getFormHelpers<AccountFormValues>(form, "username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
            fullWidth
            label={Language.usernameLabel}
            variant="outlined"
          />

          <div>
            <LoadingButton color="primary" loading={isLoading} type="submit" variant="contained">
              {isLoading ? "" : Language.updatePreferences}
            </LoadingButton>
          </div>
        </FormStack>
      </form>
    </>
  )
}
