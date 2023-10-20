import TextField from "@mui/material/TextField";
import { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { FormikContextType, FormikTouched, useFormik } from "formik";
import { FC } from "react";
import {
  getFormHelpers,
  nameValidator,
  templateDisplayNameValidator,
  onChangeTrimmed,
  iconValidator,
} from "utils/formUtils";
import * as Yup from "yup";
import { LazyIconField } from "components/IconField/LazyIconField";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import { Stack } from "components/Stack/Stack";
import Checkbox from "@mui/material/Checkbox";
import {
  HelpTooltip,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import { makeStyles } from "@mui/styles";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    name: nameValidator("Name"),
    display_name: templateDisplayNameValidator("Display name"),
    description: Yup.string().max(
      MAX_DESCRIPTION_CHAR_LIMIT,
      "Please enter a description that is less than or equal to 128 characters.",
    ),
    allow_user_cancel_workspace_jobs: Yup.boolean(),
    icon: iconValidator,
    require_active_version: Yup.boolean(),
  });

export interface TemplateSettingsForm {
  template: Template;
  onSubmit: (data: UpdateTemplateMeta) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  error?: unknown;
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>;
  accessControlEnabled: boolean;
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
  accessControlEnabled,
}) => {
  const validationSchema = getValidationSchema();
  const form: FormikContextType<UpdateTemplateMeta> =
    useFormik<UpdateTemplateMeta>({
      initialValues: {
        name: template.name,
        display_name: template.display_name,
        description: template.description,
        icon: template.icon,
        allow_user_cancel_workspace_jobs:
          template.allow_user_cancel_workspace_jobs,
        update_workspace_last_used_at: false,
        update_workspace_dormant_at: false,
        require_active_version: template.require_active_version,
      },
      validationSchema,
      onSubmit,
      initialTouched,
    });
  const getFieldHelpers = getFormHelpers(form, error);
  const styles = useStyles();

  return (
    <HorizontalForm
      onSubmit={form.handleSubmit}
      aria-label="Template settings form"
    >
      <FormSection
        title="General info"
        description="The name is used to identify the template in URLs and the API."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label="Name"
          />
        </FormFields>
      </FormSection>

      <FormSection
        title="Display info"
        description="A friendly name, description, and icon to help developers identify your template."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("display_name")}
            disabled={isSubmitting}
            fullWidth
            label="Display name"
          />

          <TextField
            {...getFieldHelpers("description")}
            multiline
            disabled={isSubmitting}
            fullWidth
            label="Description"
            rows={2}
          />

          <LazyIconField
            {...getFieldHelpers("icon")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            fullWidth
            label="Icon"
            onPickEmoji={(value) => form.setFieldValue("icon", value)}
          />
        </FormFields>
      </FormSection>

      <FormSection
        title="Operations"
        description="Regulate actions allowed on workspaces created from this template."
      >
        <Stack direction="column" spacing={5}>
          <label htmlFor="allow_user_cancel_workspace_jobs">
            <Stack direction="row" spacing={1}>
              <Checkbox
                id="allow_user_cancel_workspace_jobs"
                name="allow_user_cancel_workspace_jobs"
                disabled={isSubmitting}
                checked={form.values.allow_user_cancel_workspace_jobs}
                onChange={form.handleChange}
              />

              <Stack direction="column" spacing={0.5}>
                <Stack
                  direction="row"
                  alignItems="center"
                  spacing={0.5}
                  className={styles.optionText}
                >
                  Allow users to cancel in-progress workspace jobs.
                  <HelpTooltip>
                    <HelpTooltipText>
                      If checked, users may be able to corrupt their workspace.
                    </HelpTooltipText>
                  </HelpTooltip>
                </Stack>
                <span className={styles.optionHelperText}>
                  Depending on your template, canceling builds may leave
                  workspaces in an unhealthy state. This option isn&apos;t
                  recommended for most use cases.
                </span>
              </Stack>
            </Stack>
          </label>
          {accessControlEnabled && (
            <label htmlFor="require_active_version">
              <Stack direction="row" spacing={1}>
                <Checkbox
                  id="require_active_version"
                  name="require_active_version"
                  checked={form.values.require_active_version}
                  onChange={form.handleChange}
                />

                <Stack direction="column" spacing={0.5}>
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={0.5}
                    className={styles.optionText}
                  >
                    Require the active template version for workspace builds.
                    <HelpTooltip>
                      <HelpTooltipText>
                        This setting is not enforced for template admins.
                      </HelpTooltipText>
                    </HelpTooltip>
                  </Stack>
                  <span className={styles.optionHelperText}>
                    Workspaces that are manually started or auto-started will
                    use the promoted template version.
                  </span>
                </Stack>
              </Stack>
            </label>
          )}
        </Stack>
      </FormSection>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};

const useStyles = makeStyles((theme) => ({
  optionText: {
    fontSize: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  optionHelperText: {
    fontSize: theme.spacing(1.5),
    color: theme.palette.text.secondary,
  },
}));
