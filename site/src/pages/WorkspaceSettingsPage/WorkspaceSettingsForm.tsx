import TextField from "@mui/material/TextField";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { useFormik } from "formik";
import { type FC, useState } from "react";
import * as Yup from "yup";
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
} from "utils/formUtils";
import { Workspace } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";

export type WorkspaceSettingsFormValues = {
  name: string;
};

export const WorkspaceSettingsForm: FC<{
  isSubmitting: boolean;
  workspace: Workspace;
  error: unknown;
  onCancel: () => void;
  onSubmit: (values: WorkspaceSettingsFormValues) => void;
}> = ({ onCancel, onSubmit, workspace, error, isSubmitting }) => {
  const form = useFormik<WorkspaceSettingsFormValues>({
    onSubmit,
    initialValues: {
      name: workspace.name,
    },
    validationSchema: Yup.object({
      name: nameValidator("Name"),
    }),
  });
  const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValues>(
    form,
    error,
  );

  // We can't use `form.touched` unfortunately, because it only gets updated
  // when a field loses focus, not when it gets changed.
  const [nameModified, setNameModified] = useState(false);

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      <FormSection title="General" description="The name of your workspace.">
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={form.isSubmitting}
            onChange={onChangeTrimmed(form, (value) =>
              setNameModified(value !== form.initialValues.name),
            )}
            autoFocus
            fullWidth
            label="Name"
          />
          {nameModified && (
            <Alert severity="warning">
              Depending on the template, renaming your workspace may be
              destructive
            </Alert>
          )}
        </FormFields>
      </FormSection>
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};
