import TextField from "@mui/material/TextField";
import { FormikContextType, useFormik } from "formik";
import { FC } from "react";
import * as Yup from "yup";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";
import { Alert } from "components/Alert/Alert";
import { getFormHelpers } from "utils/formUtils";
import LoadingButton from "@mui/lab/LoadingButton";

interface SecurityFormValues {
  old_password: string;
  password: string;
  confirm_password: string;
}

export const Language = {
  oldPasswordLabel: "Old Password",
  newPasswordLabel: "New Password",
  confirmPasswordLabel: "Confirm Password",
  oldPasswordRequired: "Old password is required",
  newPasswordRequired: "New password is required",
  confirmPasswordRequired: "Password confirmation is required",
  passwordMinLength: "Password must be at least 8 characters",
  passwordMaxLength: "Password must be no more than 64 characters",
  confirmPasswordMatch: "Password and confirmation must match",
  updatePassword: "Update password",
};

const validationSchema = Yup.object({
  old_password: Yup.string().trim().required(Language.oldPasswordRequired),
  password: Yup.string()
    .trim()
    .min(8, Language.passwordMinLength)
    .max(64, Language.passwordMaxLength)
    .required(Language.newPasswordRequired),
  confirm_password: Yup.string()
    .trim()
    .test("passwords-match", Language.confirmPasswordMatch, function (value) {
      return (this.parent as SecurityFormValues).password === value;
    }),
});

export interface SecurityFormProps {
  disabled: boolean;
  isLoading: boolean;
  onSubmit: (values: SecurityFormValues) => void;
  error?: unknown;
}

export const SecurityForm: FC<SecurityFormProps> = ({
  disabled,
  isLoading,
  onSubmit,
  error,
}) => {
  const form: FormikContextType<SecurityFormValues> =
    useFormik<SecurityFormValues>({
      initialValues: {
        old_password: "",
        password: "",
        confirm_password: "",
      },
      validationSchema,
      onSubmit,
    });
  const getFieldHelpers = getFormHelpers<SecurityFormValues>(form, error);

  if (disabled) {
    return (
      <Alert severity="info">
        Password changes are only allowed for password based accounts.
      </Alert>
    );
  }

  return (
    <>
      <Form onSubmit={form.handleSubmit}>
        <FormFields>
          {Boolean(error) && <ErrorAlert error={error} />}
          <TextField
            {...getFieldHelpers("old_password")}
            autoComplete="old_password"
            fullWidth
            label={Language.oldPasswordLabel}
            type="password"
          />
          <TextField
            {...getFieldHelpers("password")}
            autoComplete="password"
            fullWidth
            label={Language.newPasswordLabel}
            type="password"
          />
          <TextField
            {...getFieldHelpers("confirm_password")}
            autoComplete="confirm_password"
            fullWidth
            label={Language.confirmPasswordLabel}
            type="password"
          />

          <div>
            <LoadingButton
              loading={isLoading}
              type="submit"
              variant="contained"
            >
              {Language.updatePassword}
            </LoadingButton>
          </div>
        </FormFields>
      </Form>
    </>
  );
};
