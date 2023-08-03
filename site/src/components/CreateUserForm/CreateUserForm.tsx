import TextField from "@mui/material/TextField"
import { FormikContextType, useFormik } from "formik"
import { FC } from "react"
import * as Yup from "yup"
import * as TypesGen from "../../api/typesGenerated"
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "../../utils/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import { hasApiFieldErrors, isApiError } from "api/errors"

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  createUser: "Create",
  cancel: "Cancel",
}

export interface CreateUserFormProps {
  onSubmit: (user: TypesGen.CreateUserRequest) => void
  onCancel: () => void
  error?: unknown
  isLoading: boolean
  myOrgId: string
}

const validationSchema = Yup.object({
  email: Yup.string()
    .trim()
    .email(Language.emailInvalid)
    .required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
})

export const CreateUserForm: FC<
  React.PropsWithChildren<CreateUserFormProps>
> = ({ onSubmit, onCancel, error, isLoading, myOrgId }) => {
  const form: FormikContextType<TypesGen.CreateUserRequest> =
    useFormik<TypesGen.CreateUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        organization_id: myOrgId,
        disable_login: false,
      },
      validationSchema,
      onSubmit,
    })
  const getFieldHelpers = getFormHelpers<TypesGen.CreateUserRequest>(
    form,
    error,
  )

  return (
    <FullPageForm title="Create user">
      {isApiError(error) && !hasApiFieldErrors(error) && (
        <ErrorAlert error={error} sx={{ mb: 4 }} />
      )}
      <form onSubmit={form.handleSubmit} autoComplete="off">
        <Stack spacing={2.5}>
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
            autoFocus
            fullWidth
            label={Language.usernameLabel}
          />
          <TextField
            {...getFieldHelpers("email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
          />
          <TextField
            {...getFieldHelpers("password")}
            autoComplete="current-password"
            fullWidth
            id="password"
            label={Language.passwordLabel}
            type="password"
          />
        </Stack>
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </form>
    </FullPageForm>
  )
}
