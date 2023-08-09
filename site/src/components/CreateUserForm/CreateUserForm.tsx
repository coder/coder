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
import Select from "@mui/material/Select"
import MenuItem from "@mui/material/MenuItem"

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
  authMethods?: TypesGen.AuthMethods
}

const validationSchema = Yup.object({
  email: Yup.string()
    .trim()
    .email(Language.emailInvalid)
    .required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
})

const authMethodSelect = (title: string, value: string) => {
  return <MenuItem value={value}>{title}</MenuItem>
}

export const CreateUserForm: FC<
  React.PropsWithChildren<CreateUserFormProps>
> = ({ onSubmit, onCancel, error, isLoading, myOrgId, authMethods }) => {
  const form: FormikContextType<TypesGen.CreateUserRequest> =
    useFormik<TypesGen.CreateUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        organization_id: myOrgId,
        disable_login: false,
        login_type: "password",
      },
      validationSchema,
      onSubmit,
    })
  const getFieldHelpers = getFormHelpers<TypesGen.CreateUserRequest>(
    form,
    error,
  )

  const methods = []
  if (authMethods?.password.enabled) {
    methods.push(authMethodSelect("Password", "password"))
  }
  if (authMethods?.oidc.enabled) {
    methods.push(authMethodSelect("OIDC", "oidc"))
  }
  if (authMethods?.github.enabled) {
    methods.push(authMethodSelect("Github", "github"))
  }
  methods.push(authMethodSelect("None", "none"))

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
          <Select
            // {...getFieldHelpers("userLoginType", "ss")}
            labelId="user-login-type"
            id="login_type"
            value={form.values.login_type}
            label="Login Type"
            onChange={form.handleChange}
          >
            {methods}
          </Select>
        </Stack>
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </form>
    </FullPageForm>
  )
}
