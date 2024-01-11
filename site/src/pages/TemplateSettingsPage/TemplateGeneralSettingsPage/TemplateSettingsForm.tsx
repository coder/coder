import { type Interpolation, type Theme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { type FormikContextType, type FormikTouched, useFormik } from "formik";
import { type FC } from "react";
import {
  getFormHelpers,
  nameValidator,
  templateDisplayNameValidator,
  onChangeTrimmed,
  iconValidator,
} from "utils/formUtils";
import * as Yup from "yup";
import { IconField } from "components/IconField/IconField";
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
  HelpTooltipContent,
  HelpTooltipText,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { EnterpriseBadge } from "components/Badges/Badges";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE =
  "Please enter a description that is no longer than 128 characters.";

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    name: nameValidator("Name"),
    display_name: templateDisplayNameValidator("Display name"),
    description: Yup.string().max(
      MAX_DESCRIPTION_CHAR_LIMIT,
      MAX_DESCRIPTION_MESSAGE,
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
        deprecation_message: template.deprecation_message,
        disable_everyone_group_access: false,
      },
      validationSchema,
      onSubmit,
      initialTouched,
    });
  const getFieldHelpers = getFormHelpers(form, error);

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
            {...getFieldHelpers("description", {
              maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
            })}
            multiline
            disabled={isSubmitting}
            fullWidth
            label="Description"
            rows={2}
          />

          <IconField
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
                  css={styles.optionText}
                >
                  Allow users to cancel in-progress workspace jobs.
                  <HelpTooltip>
                    <HelpTooltipTrigger />
                    <HelpTooltipContent>
                      <HelpTooltipText>
                        If checked, users may be able to corrupt their
                        workspace.
                      </HelpTooltipText>
                    </HelpTooltipContent>
                  </HelpTooltip>
                </Stack>
                <span css={styles.optionHelperText}>
                  Depending on your template, canceling builds may leave
                  workspaces in an unhealthy state. This option isn&apos;t
                  recommended for most use cases.
                </span>
              </Stack>
            </Stack>
          </label>
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
                  css={styles.optionText}
                >
                  Require workspaces automatically update when started.
                  <HelpTooltip>
                    <HelpTooltipTrigger />
                    <HelpTooltipContent>
                      <HelpTooltipText>
                        This setting is not enforced for template admins.
                      </HelpTooltipText>
                    </HelpTooltipContent>
                  </HelpTooltip>
                </Stack>
                <span css={styles.optionHelperText}>
                  Workspaces that are manually started or auto-started will use
                  the active template version.
                </span>
              </Stack>
            </Stack>
          </label>
        </Stack>
      </FormSection>

      <FormSection
        title="Deprecate"
        description="Deprecating a template prevents any new workspaces from being created. Existing workspaces will continue to function."
      >
        <FormFields>
          <Stack direction="column" spacing={0.5}>
            <Stack
              direction="row"
              alignItems="center"
              spacing={0.5}
              css={styles.optionText}
            >
              Deprecation Message
            </Stack>
            <span css={styles.optionHelperText}>
              Leave the message empty to keep the template active. Any message
              provided will mark the template as deprecated. Use this message to
              inform users of the deprecation and how to migrate to a new
              template.
            </span>
          </Stack>
          <TextField
            {...getFieldHelpers("deprecation_message")}
            disabled={isSubmitting || !accessControlEnabled}
            fullWidth
            label="Deprecation Message"
          />
          {!accessControlEnabled && (
            <Stack direction="row">
              <EnterpriseBadge />
              <span css={styles.optionHelperText}>
                Enterprise license required to deprecate templates.
              </span>
            </Stack>
          )}
        </FormFields>
      </FormSection>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};

const styles = {
  optionText: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.primary,
  }),

  optionHelperText: (theme) => ({
    fontSize: 12,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
