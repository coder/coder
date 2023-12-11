import TextField from "@mui/material/TextField";
import LoadingButton from "@mui/lab/LoadingButton";
import { type FormikTouched, useFormik } from "formik";
import { type FC } from "react";
import * as Yup from "yup";
import type { UpdateUserThemePreferenceRequest } from "api/typesGenerated";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";

const validationSchema = Yup.object({
  theme_preference: Yup.string().required(),
});

export interface AppearanceFormProps {
  isLoading: boolean;
  error?: unknown;
  initialValues: UpdateUserThemePreferenceRequest;
  onSubmit: (values: UpdateUserThemePreferenceRequest) => void;
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<UpdateUserThemePreferenceRequest>;
}

export const AppearanceForm: FC<AppearanceFormProps> = ({
  isLoading,
  error,
  onSubmit,
  initialValues,
  initialTouched,
}) => {
  const form = useFormik({
    initialValues,
    validationSchema,
    onSubmit,
    initialTouched,
  });
  const getFieldHelpers = getFormHelpers(form, error);

  return (
    <>
      <Form onSubmit={form.handleSubmit}>
        <FormFields>
          {Boolean(error) && <ErrorAlert error={error} />}
          <TextField
            {...getFieldHelpers("theme_preference")}
            onChange={onChangeTrimmed(form)}
            fullWidth
            label="Theme name"
          />

          <div>
            <LoadingButton
              loading={isLoading}
              type="submit"
              variant="contained"
            >
              Update theme
            </LoadingButton>
          </div>
        </FormFields>
      </Form>
    </>
  );
};
