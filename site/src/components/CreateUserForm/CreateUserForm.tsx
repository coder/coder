import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { CreateUserRequest } from "../../api/typesGenerated"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"

const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  usernameRequired: "Please enter a username.",
  createUser: "Create",
  cancel: "Cancel",
}

export interface CreateUserFormProps {
  onSubmit: (user: CreateUserRequest) => void
  onCancel: () => void
  formErrors?: FormikErrors<CreateUserRequest>
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  password: Yup.string().required(),
  username: Yup.string().required(),
})

export const CreateUserForm: React.FC<CreateUserFormProps> = ({ onSubmit, onCancel, formErrors }) => {
  const form: FormikContextType<CreateUserRequest> = useFormik<CreateUserRequest>({
    initialValues: {
      email: "",
      password: "",
      username: "",
    },
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateUserRequest>(form, formErrors)

  return (
    <form onSubmit={form.handleSubmit}>
      <TextField
        {...getFieldHelpers("username")}
        onChange={onChangeTrimmed(form)}
        autoFocus
        autoComplete="username"
        fullWidth
        label={Language.usernameLabel}
        variant="outlined"
      />
      <TextField
        {...getFieldHelpers("email")}
        onChange={onChangeTrimmed(form)}
        autoFocus
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
      <div>
        <Button onClick={onCancel}>{Language.cancel}</Button>
        <LoadingButton color="primary" type="submit" variant="contained">
          {Language.createUser}
        </LoadingButton>
      </div>
    </form>
  )
}
