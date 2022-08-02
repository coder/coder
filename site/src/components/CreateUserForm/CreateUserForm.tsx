import FormHelperText from "@material-ui/core/FormHelperText"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import { FC } from "react"
import * as Yup from "yup"
import * as TypesGen from "../../api/typesGenerated"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "../../util/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"

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
  formErrors?: FormikErrors<TypesGen.CreateUserRequest>
  isLoading: boolean
  error?: string
  myOrgId: string
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
})

export const CreateUserForm: FC<React.PropsWithChildren<CreateUserFormProps>> = ({
  onSubmit,
  onCancel,
  formErrors,
  isLoading,
  error,
  myOrgId,
}) => {
  const form: FormikContextType<TypesGen.CreateUserRequest> = useFormik<TypesGen.CreateUserRequest>(
    {
      initialValues: {
        email: "",
        password: "",
        username: "",
        organization_id: myOrgId,
      },
      validationSchema,
      onSubmit,
    },
  )
  const getFieldHelpers = getFormHelpers<TypesGen.CreateUserRequest>(form, formErrors)

  return (
    <FullPageForm title="Create user" onCancel={onCancel}>
      <form onSubmit={form.handleSubmit}>
        <Stack spacing={1}>
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
            autoFocus
            fullWidth
            label={Language.usernameLabel}
            variant="outlined"
          />
          <TextField
            {...getFieldHelpers("email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
            variant="outlined"
          />
          <TextField
            {...getFieldHelpers("password")}
            autoComplete="current-password"
            fullWidth
            id="password"
            label={Language.passwordLabel}
            type="password"
            variant="outlined"
          />
        </Stack>
        {error && <FormHelperText error>{error}</FormHelperText>}
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </form>
    </FullPageForm>
  )
}
