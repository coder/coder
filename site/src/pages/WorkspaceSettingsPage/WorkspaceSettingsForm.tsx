import TextField from "@mui/material/TextField";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { useFormik } from "formik";
import { type FC } from "react";
import * as Yup from "yup";
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
} from "utils/formUtils";
import {
  AutomaticUpdates,
  AutomaticUpdateses,
  Workspace,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import MenuItem from "@mui/material/MenuItem";
import { useTemplatePoliciesEnabled } from "components/Dashboard/DashboardProvider";
import upperFirst from "lodash/upperFirst";

export type WorkspaceSettingsFormValues = {
  name: string;
  automatic_updates: AutomaticUpdates;
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
      automatic_updates: workspace.automatic_updates,
    },
    validationSchema: Yup.object({
      name: nameValidator("Name"),
      automatic_updates: Yup.string().oneOf(AutomaticUpdateses),
    }),
  });
  const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValues>(
    form,
    error,
  );

  const templatePoliciesEnabled = useTemplatePoliciesEnabled();

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      <FormSection
        title="Workspace Name"
        description="Update the name of your workspace."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={form.isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label="Name"
          />
          {form.values.name !== form.initialValues.name && (
            <Alert severity="warning">
              Depending on the template, renaming your workspace may be
              destructive
            </Alert>
          )}
        </FormFields>
      </FormSection>
      {templatePoliciesEnabled && (
        <FormSection
          title="Automatic Updates"
          description="Configure your workspace to automatically update when started."
        >
          <FormFields>
            <TextField
              {...getFieldHelpers("automatic_updates")}
              id="automatic_updates"
              label="Update Policy"
              value={form.values.automatic_updates}
              select
              disabled={form.isSubmitting}
            >
              {AutomaticUpdateses.map((value) => (
                <MenuItem value={value} key={value}>
                  {upperFirst(value)}
                </MenuItem>
              ))}
            </TextField>
          </FormFields>
        </FormSection>
      )}
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};
