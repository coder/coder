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
import MenuItem from "@mui/material/MenuItem";
import upperFirst from "lodash/upperFirst";
import { type Theme } from "@emotion/react";

export type WorkspaceSettingsFormValues = {
  name: string;
  automatic_updates: AutomaticUpdates;
};

export const WorkspaceSettingsForm: FC<{
  workspace: Workspace;
  error: unknown;
  onCancel: () => void;
  onSubmit: (values: WorkspaceSettingsFormValues) => Promise<void>;
}> = ({ onCancel, onSubmit, workspace, error }) => {
  const formEnabled =
    !workspace.template_require_active_version || workspace.allow_renames;

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

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      <FormSection
        title="Workspace Name"
        description="Update the name of your workspace."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={!workspace.allow_renames || form.isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label="Name"
            css={workspace.allow_renames && styles.nameWarning}
            helperText={
              workspace.allow_renames
                ? form.values.name !== form.initialValues.name &&
                  "Depending on the template, renaming your workspace may be destructive"
                : "Renaming your workspace can be destructive and has not been enabled for this deployment."
            }
          />
        </FormFields>
      </FormSection>
      <FormSection
        title="Automatic Updates"
        description="Configure your workspace to automatically update when started."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("automatic_updates")}
            id="automatic_updates"
            label="Update Policy"
            value={
              workspace.template_require_active_version
                ? "always"
                : form.values.automatic_updates
            }
            select
            disabled={
              form.isSubmitting || workspace.template_require_active_version
            }
            helperText={
              workspace.template_require_active_version &&
              "The template for this workspace requires automatic updates."
            }
          >
            {AutomaticUpdateses.map((value) => (
              <MenuItem value={value} key={value}>
                {upperFirst(value)}
              </MenuItem>
            ))}
          </TextField>
        </FormFields>
      </FormSection>
      {formEnabled && (
        <FormFooter onCancel={onCancel} isLoading={form.isSubmitting} />
      )}
    </HorizontalForm>
  );
};

const styles = {
  nameWarning: (theme: Theme) => ({
    "& .MuiFormHelperText-root": {
      color: theme.palette.warning.light,
    },
  }),
};
