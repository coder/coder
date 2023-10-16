import TextField from "@mui/material/TextField";
import { FormikTouched, useFormik } from "formik";
import { FC } from "react";
import * as Yup from "yup";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { LoadingButton } from "components/LoadingButton/LoadingButton";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";
import { UpdateUserProfileRequest } from "api/typesGenerated";

export const Language = {
  usernameLabel: "Username",
  emailLabel: "Email",
  updateSettings: "Update account",
};

const validationSchema = Yup.object({
  username: nameValidator(Language.usernameLabel),
});

export interface AccountFormProps {
  editable: boolean;
  email: string;
  isLoading: boolean;
  initialValues: UpdateUserProfileRequest;
  onSubmit: (values: UpdateUserProfileRequest) => void;
  updateProfileError?: unknown;
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<UpdateUserProfileRequest>;
}

export const AccountForm: FC<AccountFormProps> = ({
  editable,
  email,
  isLoading,
  onSubmit,
  initialValues,
  updateProfileError,
  initialTouched,
}) => {
  const form = useFormik({
    initialValues,
    validationSchema,
    onSubmit,
    initialTouched,
  });
  const getFieldHelpers = getFormHelpers(form, updateProfileError);

  return (
    <>
      <Form onSubmit={form.handleSubmit}>
        <FormFields>
          {Boolean(updateProfileError) && (
            <ErrorAlert error={updateProfileError} />
          )}
          <TextField
            disabled
            fullWidth
            label={Language.emailLabel}
            value={email}
          />
          <TextField
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            aria-disabled={!editable}
            autoComplete="username"
            disabled={!editable}
            fullWidth
            label={Language.usernameLabel}
          />

          <div>
            <LoadingButton
              loading={isLoading}
              aria-disabled={!editable}
              disabled={!editable}
              type="submit"
              variant="contained"
            >
              {isLoading ? "" : Language.updateSettings}
            </LoadingButton>
          </div>
        </FormFields>
      </Form>
    </>
  );
};
