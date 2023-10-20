import TextField from "@mui/material/TextField";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { useFormik } from "formik";
import { useState, type FC } from "react";
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
import {
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  SelectChangeEvent,
} from "@mui/material";

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
    }),
  });
  const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValues>(
    form,
    error,
  );

  const capitalizeFirstLetter = (inputString: string): string => {
    return inputString.charAt(0).toUpperCase() + inputString.slice(1);
  };

  const [automaticUpdates, setAutomaticUpdates] = useState<AutomaticUpdates>(
    workspace.automatic_updates,
  );

  const handleChange = (event: SelectChangeEvent) => {
    setAutomaticUpdates(event.target.value as AutomaticUpdates);
    form.setFieldValue("automatic_updates", automaticUpdates);
  };

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
      <FormSection
        title="Automatic Updates"
        description="Configure your workspace to automatically update to the active template version when started."
      >
        <FormControl fullWidth>
          <FormFields>
            <InputLabel htmlFor="automatic_updates">Update Policy</InputLabel>
            <Select
              labelId="automatic_updates"
              id="automatic_updates"
              label="Update Policy"
              value={automaticUpdates}
              onChange={handleChange}
              disabled={form.isSubmitting}
            >
              {AutomaticUpdateses.map((value) => (
                <MenuItem value={value}>
                  {capitalizeFirstLetter(value)}
                </MenuItem>
              ))}
            </Select>
          </FormFields>
        </FormControl>
      </FormSection>
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};
