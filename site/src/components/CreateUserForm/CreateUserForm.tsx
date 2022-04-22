import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, useFormik } from "formik"
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
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  password: Yup.string().required(),
  username: Yup.string().required(),
})

export const CreateUserForm: React.FC<CreateUserFormProps> = ({ onSubmit }) => {
  const form: FormikContextType<CreateUserRequest> = useFormik<CreateUserRequest>({
    initialValues: {
      email: "",
      password: "",
      username: "",
    },
    validationSchema,
    onSubmit,
  })

  return (
    <form onSubmit={form.handleSubmit}>
      <TextField
        {...getFormHelpers<CreateUserRequest>(form, "username")}
        onChange={onChangeTrimmed(form)}
        autoFocus
        autoComplete="username"
        fullWidth
        label={Language.usernameLabel}
        variant="outlined"
      />
      <TextField
        {...getFormHelpers<CreateUserRequest>(form, "email")}
        onChange={onChangeTrimmed(form)}
        autoFocus
        autoComplete="email"
        fullWidth
        label={Language.emailLabel}
        variant="outlined"
      />
      <TextField
        {...getFormHelpers<CreateUserRequest>(form, "password")}
        autoComplete="current-password"
        fullWidth
        id="password"
        label={Language.passwordLabel}
        type="password"
        variant="outlined"
      />
      <div>
        <LoadingButton color="primary" type="submit" variant="contained">
          {Language.createUser}
        </LoadingButton>
        <Button>{Language.cancel}</Button>
      </div>
    </form>
  )
}
