import Checkbox from "@mui/material/Checkbox";
import TextField from "@mui/material/TextField";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { type FormikContextType, useFormik } from "formik";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import type * as TypesGen from "api/typesGenerated";
import LoadingButton from "@mui/lab/LoadingButton";
import { FormFields, VerticalForm } from "components/Form/Form";
import { CoderIcon } from "components/Icons/CoderIcon";
import Link from "@mui/material/Link";
import { docs } from "utils/docs";

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  create: "Create account",
  welcomeMessage: <>Welcome to Coder</>,
};

const validationSchema = Yup.object({
  email: Yup.string()
    .trim()
    .email(Language.emailInvalid)
    .required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
});

export interface SetupPageViewProps {
  onSubmit: (firstUser: TypesGen.CreateFirstUserRequest) => void;
  error?: unknown;
  isLoading?: boolean;
}

export const SetupPageView: React.FC<SetupPageViewProps> = ({
  onSubmit,
  error,
  isLoading,
}) => {
  const form: FormikContextType<TypesGen.CreateFirstUserRequest> =
    useFormik<TypesGen.CreateFirstUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        trial: false,
      },
      validationSchema,
      onSubmit,
    });
  const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
    form,
    error,
  );

  return (
    <SignInLayout>
      <header css={{ textAlign: "center", marginBottom: 32 }}>
        <CoderIcon
          css={(theme) => ({
            color: theme.palette.text.primary,
            fontSize: 64,
          })}
        />
        <h1
          css={{
            fontWeight: 400,
            margin: 0,
            marginTop: 16,
          }}
        >
          Welcome to <strong>Coder</strong>
        </h1>
        <div
          css={(theme) => ({
            marginTop: 12,
            color: theme.palette.text.secondary,
          })}
        >
          Let&lsquo;s create your first admin user account
        </div>
      </header>
      <VerticalForm onSubmit={form.handleSubmit}>
        <FormFields>
          <TextField
            autoFocus
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
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

          <label
            htmlFor="trial"
            css={{
              display: "flex",
              cursor: "pointer",
              alignItems: "flex-start",
              gap: 4,
              marginTop: -4,
              marginBottom: 8,
            }}
          >
            <Checkbox
              id="trial"
              name="trial"
              checked={form.values.trial}
              onChange={form.handleChange}
              data-testid="trial"
              size="small"
            />

            <div css={{ fontSize: 14, paddingTop: 4 }}>
              <span css={{ display: "block", fontWeight: 600 }}>
                Start a 30-day free trial of Enterprise
              </span>
              <span
                css={(theme) => ({
                  display: "block",
                  fontSize: 13,
                  color: theme.palette.text.secondary,
                  lineHeight: "1.6",
                })}
              >
                Get access to high availability, template RBAC, audit logging,
                quotas, and more.
              </span>
              <Link
                href={docs("/enterprise")}
                target="_blank"
                css={{ marginTop: 4, display: "inline-block", fontSize: 13 }}
              >
                Read more
              </Link>
            </div>
          </label>

          <LoadingButton
            fullWidth
            loading={isLoading}
            type="submit"
            data-testid="create"
            size="large"
            variant="contained"
            color="primary"
          >
            {Language.create}
          </LoadingButton>
        </FormFields>
      </VerticalForm>
    </SignInLayout>
  );
};
