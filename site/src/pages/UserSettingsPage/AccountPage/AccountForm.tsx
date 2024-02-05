import TextField from "@mui/material/TextField";
import { FormikTouched, useFormik } from "formik";
import { FC } from "react";
import * as Yup from "yup";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";
import { UpdateUserProfileRequest } from "api/typesGenerated";
import LoadingButton from "@mui/lab/LoadingButton";

export const Language = {
  usernameLabel: "Username",
  emailLabel: "Email",
  nameLabel: "Name",
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
        <TextField
          {...getFieldHelpers("name")}
          onBlur={(e) => {
            e.target.value = e.target.value.trim();
            form.handleChange(e);
          }}
          aria-disabled={!editable}
          disabled={!editable}
          fullWidth
          label={Language.nameLabel}
          helperText='The human-readable name is optional and can be accessed in a template via the "data.coder_workspace.me.owner_name" property.'
        />

        <div>
          <LoadingButton
            loading={isLoading}
            disabled={!editable}
            type="submit"
            variant="contained"
          >
            {Language.updateSettings}
          </LoadingButton>
        </div>
      </FormFields>
    </Form>
  );
};
