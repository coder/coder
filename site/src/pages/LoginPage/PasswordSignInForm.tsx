import { Stack } from "components/Stack/Stack";
import TextField from "@mui/material/TextField";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import { LoadingButton } from "components/LoadingButton/LoadingButton";
import { Language } from "./SignInForm";
import { FormikContextType, FormikTouched, useFormik } from "formik";
import * as Yup from "yup";
import { FC } from "react";
import { BuiltInAuthFormValues } from "./SignInForm.types";

type PasswordSignInFormProps = {
  onSubmit: (credentials: { email: string; password: string }) => void;
  initialTouched?: FormikTouched<BuiltInAuthFormValues>;
  isSigningIn: boolean;
};

export const PasswordSignInForm: FC<PasswordSignInFormProps> = ({
  onSubmit,
  initialTouched,
  isSigningIn,
}) => {
  const validationSchema = Yup.object({
    email: Yup.string()
      .trim()
      .email(Language.emailInvalid)
      .required(Language.emailRequired),
    password: Yup.string(),
  });

  const form: FormikContextType<BuiltInAuthFormValues> =
    useFormik<BuiltInAuthFormValues>({
      initialValues: {
        email: "",
        password: "",
      },
      validationSchema,
      onSubmit,
      initialTouched,
    });
  const getFieldHelpers = getFormHelpers<BuiltInAuthFormValues>(form);

  return (
    <form onSubmit={form.handleSubmit}>
      <Stack spacing={2.5}>
        <TextField
          {...getFieldHelpers("email")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          autoComplete="email"
          fullWidth
          label={Language.emailLabel}
          type="email"
        />
        <TextField
          {...getFieldHelpers("password")}
          autoComplete="current-password"
          fullWidth
          id="password"
          label={Language.passwordLabel}
          type="password"
        />
        <div>
          <LoadingButton
            size="large"
            loading={isSigningIn}
            fullWidth
            type="submit"
          >
            {isSigningIn ? "" : Language.passwordSignIn}
          </LoadingButton>
        </div>
      </Stack>
    </form>
  );
};
