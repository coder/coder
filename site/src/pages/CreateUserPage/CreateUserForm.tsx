import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import { type FormikContextType, useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { hasApiFieldErrors, isApiError } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { FormFooter } from "components/FormFooter/FormFooter";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { Stack } from "components/Stack/Stack";
import {
  displayNameValidator,
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  nameLabel: "Full name",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  createUser: "Create",
  cancel: "Cancel",
};

export const authMethodLanguage = {
  password: {
    displayName: "Password",
    description: "Use an email address and password to login",
  },
  oidc: {
    displayName: "OpenID Connect",
    description: "Use an OpenID Connect provider for authentication",
  },
  github: {
    displayName: "Github",
    description: "Use Github OAuth for authentication",
  },
  none: {
    displayName: "None",
    description: (
      <>
        Disable authentication for this user (See the{" "}
        <Link
          target="_blank"
          rel="noopener"
          href="https://coder.com/docs/v2/latest/admin/auth#disable-built-in-authentication"
        >
          documentation
        </Link>{" "}
        for more details)
      </>
    ),
  },
};

export interface CreateUserFormProps {
  onSubmit: (user: TypesGen.CreateUserRequest) => void;
  onCancel: () => void;
  error?: unknown;
  isLoading: boolean;
  organizationId: string;
  authMethods?: TypesGen.AuthMethods;
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
  name: displayNameValidator(Language.nameLabel),
  login_type: Yup.string().oneOf(Object.keys(authMethodLanguage)),
});

export const CreateUserForm: FC<
  React.PropsWithChildren<CreateUserFormProps>
> = ({ onSubmit, onCancel, error, isLoading, organizationId, authMethods }) => {
  const form: FormikContextType<TypesGen.CreateUserRequest> =
    useFormik<TypesGen.CreateUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        name: "",
        organization_id: organizationId,
        disable_login: false,
        login_type: "",
      },
      validationSchema,
      onSubmit,
    });
  const getFieldHelpers = getFormHelpers<TypesGen.CreateUserRequest>(
    form,
    error,
  );

  const methods = [
    authMethods?.password.enabled && "password",
    authMethods?.oidc.enabled && "oidc",
    authMethods?.github.enabled && "github",
    "none",
  ].filter(Boolean) as Array<keyof typeof authMethodLanguage>;

  return (
    <FullPageForm title="Create user">
      {isApiError(error) && !hasApiFieldErrors(error) && (
        <ErrorAlert error={error} css={{ marginBottom: 32 }} />
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
            {...getFieldHelpers("name")}
            autoComplete="name"
            fullWidth
            label={Language.nameLabel}
          />
          <TextField
            {...getFieldHelpers("email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
          />
          <TextField
            {...getFieldHelpers("login_type", {
              helperText: "Authentication method for this user",
            })}
            select
            id="login_type"
            data-testid="login-type-input"
            value={form.values.login_type}
            label="Login Type"
            onChange={async (e) => {
              if (e.target.value !== "password") {
                await form.setFieldValue("password", "");
              }
              await form.setFieldValue("login_type", e.target.value);
            }}
            SelectProps={{
              renderValue: (selected: unknown) =>
                authMethodLanguage[selected as keyof typeof authMethodLanguage]
                  ?.displayName ?? "",
            }}
          >
            {methods.map((value) => {
              const language = authMethodLanguage[value];
              return (
                <MenuItem key={value} id={"item-" + value} value={value}>
                  <Stack
                    spacing={0}
                    css={{
                      maxWidth: 400,
                    }}
                  >
                    {language.displayName}
                    <span
                      css={(theme) => ({
                        fontSize: 14,
                        color: theme.palette.text.secondary,
                        wordWrap: "normal",
                        whiteSpace: "break-spaces",
                      })}
                    >
                      {language.description}
                    </span>
                  </Stack>
                </MenuItem>
              );
            })}
          </TextField>
          <TextField
            {...getFieldHelpers("password", {
              helperText:
                form.values.login_type !== "password" &&
                "No password required for this login type",
            })}
            autoComplete="current-password"
            fullWidth
            id="password"
            data-testid="password-input"
            disabled={form.values.login_type !== "password"}
            label={Language.passwordLabel}
            type="password"
          />
        </Stack>
        <FormFooter
          submitLabel="Create user"
          onCancel={onCancel}
          isLoading={isLoading}
        />
      </form>
    </FullPageForm>
  );
};
