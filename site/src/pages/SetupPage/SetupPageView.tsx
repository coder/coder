import Box from "@material-ui/core/Box"
import Checkbox from "@material-ui/core/Checkbox"
import FormHelperText from "@material-ui/core/FormHelperText"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Typography from "@material-ui/core/Typography"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import { Stack } from "components/Stack/Stack"
import { Welcome } from "components/Welcome/Welcome"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"
import * as TypesGen from "../../api/typesGenerated"

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  create: "Setup account",
  welcomeMessage: <>Welcome to Coder</>,
}

const validationSchema = Yup.object({
  email: Yup.string()
    .trim()
    .email(Language.emailInvalid)
    .required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
})

export interface SetupPageViewProps {
  onSubmit: (firstUser: TypesGen.CreateFirstUserRequest) => void
  formErrors?: FormikErrors<TypesGen.CreateFirstUserRequest>
  genericError?: string
  isLoading?: boolean
}

export const SetupPageView: React.FC<SetupPageViewProps> = ({
  onSubmit,
  formErrors,
  genericError,
  isLoading,
}) => {
  const form: FormikContextType<TypesGen.CreateFirstUserRequest> =
    useFormik<TypesGen.CreateFirstUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        trial: true,
      },
      validationSchema,
      onSubmit,
    })
  const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
    form,
    formErrors,
  )
  const styles = useStyles()

  return (
    <SignInLayout>
      <Welcome message={Language.welcomeMessage} />
      <form onSubmit={form.handleSubmit}>
        <Stack>
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
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
          {genericError && (
            <FormHelperText error>{genericError}</FormHelperText>
          )}
          <div className={styles.callout}>
            <Box display="flex">
              <div>
                <Checkbox
                  id="trial"
                  name="trial"
                  defaultChecked
                  value={form.values.trial}
                  onChange={form.handleChange}
                />
              </div>

              <Box>
                <Typography variant="h6" style={{ fontSize: 14 }}>
                  Start a 30-day free trial of Enterprise
                </Typography>
                <Typography variant="caption" color="textSecondary">
                  Get access to high availability, template RBAC, audit logging,
                  quotas, and more.
                </Typography>
              </Box>
            </Box>
          </div>
          <LoadingButton
            fullWidth
            variant="outlined"
            loading={isLoading}
            type="submit"
          >
            {Language.create}
          </LoadingButton>
        </Stack>
      </form>
    </SignInLayout>
  )
}

const useStyles = makeStyles(() => ({
  callout: {
    borderRadius: 16,
  },
}))
