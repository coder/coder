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
  password: Yup.string().when("login_type", {
    is: "password",
    then: (schema) => schema.required(Language.passwordRequired),
    otherwise: (schema) => schema,
  }),
  username: nameValidator(Language.usernameLabel),
})

const authMethodSelect = (
  title: string,
  value: string,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- future will use this
  description: string,
) => {
  return (
    <MenuItem id={value} value={value}>
      {title}
      {/* TODO: Add description */}
    </MenuItem>
  )
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
    methods.push(
      authMethodSelect(
        "Password",
        "password",
        "User can provide their email and password to login.",
      ),
    )
  }
  if (authMethods?.oidc.enabled) {
    methods.push(
      authMethodSelect(
        "OIDC",
        "oidc",
        "Uses an OIDC provider to authenticate the user.",
      ),
    )
  }
  if (authMethods?.github.enabled) {
    methods.push(
      authMethodSelect(
        "Github",
        "github",
        "Uses github oauth to authenticate the user.",
      ),
    )
  }
  methods.push(
    authMethodSelect(
      "None",
      "none",
      "User authentication is disabled. This user an only be used if an api token is created for them.",
    ),
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
            {...getFieldHelpers(
              "password",
              form.values.login_type === "password"
                ? ""
                : "No password required for this login type",
            )}
            autoComplete="current-password"
            fullWidth
            id="password"
            data-testid="password"
            disabled={form.values.login_type !== "password"}
            label={Language.passwordLabel}
            type="password"
          />
          <TextField
            {...getFieldHelpers(
              "login_type",
              "Authentication method for this user",
            )}
            select
            id="login_type"
            value={form.values.login_type}
            label="Login Type"
            onChange={async (e) => {
              if (e.target.value !== "password") {
                await form.setFieldValue("password", "")
              }
              await form.setFieldValue("login_type", e.target.value)
            }}
          >
            {methods}
          </TextField>
        </Stack>
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </form>
    </FullPageForm>
  )
}
