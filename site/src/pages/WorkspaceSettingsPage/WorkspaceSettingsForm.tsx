import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { useFormik } from "formik";
import { FC } from "react";
import { useTranslation } from "react-i18next";
import * as Yup from "yup";
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
} from "utils/formUtils";
import TextField from "@mui/material/TextField";
import { Workspace } from "api/typesGenerated";

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
  const { t } = useTranslation("workspaceSettingsPage");

  const form = useFormik<WorkspaceSettingsFormValues>({
    onSubmit,
    initialValues: {
      name: workspace.name,
    },
    validationSchema: Yup.object({
      name: nameValidator(t("nameLabel")),
    }),
  });
  const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValues>(
    form,
    error,
  );

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      <FormSection
        title="General info"
        description="The name of your new workspace."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={form.isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label={t("nameLabel")}
          />
        </FormFields>
      </FormSection>
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};
