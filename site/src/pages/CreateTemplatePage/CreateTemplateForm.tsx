import { type Interpolation, type Theme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import { type FC, useEffect } from "react";
import camelCase from "lodash/camelCase";
import capitalize from "lodash/capitalize";
import * as Yup from "yup";
import type {
  ProvisionerJobLog,
  Template,
  TemplateExample,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate";
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
  templateDisplayNameValidator,
} from "utils/formUtils";
import {
  type TemplateAutostartRequirementDaysValue,
  type TemplateAutostopRequirementDaysValue,
} from "utils/schedule";
import { sortedDays } from "modules/templates/TemplateScheduleAutostart/TemplateScheduleAutostart";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { IconField } from "components/IconField/IconField";
import {
  HorizontalForm,
  FormSection,
  FormFields,
  FormFooter,
} from "components/Form/Form";
import { TemplateUpload, type TemplateUploadProps } from "./TemplateUpload";
import { VariableInput } from "./VariableInput";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;

export interface CreateTemplateData {
  name: string;
  display_name: string;
  description: string;
  icon: string;
  default_ttl_hours: number;
  use_max_ttl: boolean;
  max_ttl_hours: number;
  autostart_requirement_days_of_week: TemplateAutostartRequirementDaysValue[];
  autostop_requirement_days_of_week: TemplateAutostopRequirementDaysValue;
  autostop_requirement_weeks: number;
  allow_user_autostart: boolean;
  allow_user_autostop: boolean;
  allow_user_cancel_workspace_jobs: boolean;
  parameter_values_by_name?: Record<string, string>;
  user_variable_values?: VariableValue[];
  allow_everyone_group_access: boolean;
}

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  display_name: templateDisplayNameValidator("Display name"),
  description: Yup.string().max(
    MAX_DESCRIPTION_CHAR_LIMIT,
    "Please enter a description that is less than or equal to 128 characters.",
  ),
  icon: Yup.string().optional(),
});

const defaultInitialValues: CreateTemplateData = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  // max_ttl is an enterprise-only feature, and the server ignores the value if
  // you are not licensed. We hide the form value based on entitlements.
  //
  // The maximum value is 30 days but we default to 7 days as it's a much more
  // sensible value for most teams.
  use_max_ttl: false, // autostop_requirement is default
  max_ttl_hours: 24 * 7,
  // autostop_requirement is an enterprise-only feature, and the server ignores
  // the value if you are not licensed. We hide the form value based on
  // entitlements.
  //
  // Default to requiring restart every Sunday in the user's quiet hours in the
  // user's timezone.
  autostop_requirement_days_of_week: "sunday",
  autostop_requirement_weeks: 1,
  autostart_requirement_days_of_week: sortedDays,
  allow_user_cancel_workspace_jobs: false,
  allow_user_autostart: false,
  allow_user_autostop: false,
  allow_everyone_group_access: true,
};

type GetInitialValuesParams = {
  fromExample?: TemplateExample;
  fromCopy?: Template;
  variables?: TemplateVersionVariable[];
  allowAdvancedScheduling: boolean;
};

const getInitialValues = ({
  fromExample,
  fromCopy,
  allowAdvancedScheduling,
  variables,
}: GetInitialValuesParams) => {
  let initialValues = defaultInitialValues;

  if (!allowAdvancedScheduling) {
    initialValues = {
      ...initialValues,
      max_ttl_hours: 0,
      autostop_requirement_days_of_week: "off",
      autostop_requirement_weeks: 1,
    };
  }

  if (fromExample) {
    initialValues = {
      ...initialValues,
      name: fromExample.id,
      display_name: fromExample.name,
      icon: fromExample.icon,
      description: fromExample.description,
    };
  }

  if (fromCopy) {
    initialValues = {
      ...initialValues,
      ...fromCopy,
      name: `${fromCopy.name}-copy`,
      display_name: fromCopy.display_name
        ? `Copy of ${fromCopy.display_name}`
        : "",
    };
  }

  if (variables) {
    variables.forEach((variable) => {
      if (!initialValues.user_variable_values) {
        initialValues.user_variable_values = [];
      }
      initialValues.user_variable_values.push({
        name: variable.name,
        value: variable.sensitive ? "" : variable.value,
      });
    });
  }

  return initialValues;
};

type CopiedTemplateForm = { copiedTemplate: Template };
type StarterTemplateForm = { starterTemplate: TemplateExample };
type UploadTemplateForm = { upload: TemplateUploadProps };

export type CreateTemplateFormProps = (
  | CopiedTemplateForm
  | StarterTemplateForm
  | UploadTemplateForm
) & {
  onCancel: () => void;
  onSubmit: (data: CreateTemplateData) => void;
  isSubmitting: boolean;
  variables?: TemplateVersionVariable[];
  error?: unknown;
  jobError?: string;
  logs?: ProvisionerJobLog[];
  allowAdvancedScheduling: boolean;
};

export const CreateTemplateForm: FC<CreateTemplateFormProps> = (props) => {
  const {
    onCancel,
    onSubmit,
    variables,
    isSubmitting,
    error,
    jobError,
    logs,
    allowAdvancedScheduling,
  } = props;
  const form = useFormik<CreateTemplateData>({
    initialValues: getInitialValues({
      allowAdvancedScheduling,
      fromExample:
        "starterTemplate" in props ? props.starterTemplate : undefined,
      fromCopy: "copiedTemplate" in props ? props.copiedTemplate : undefined,
      variables,
    }),
    validationSchema,
    onSubmit,
  });
  const getFieldHelpers = getFormHelpers<CreateTemplateData>(form, error);

  useEffect(() => {
    if (error) {
      window.scrollTo(0, 0);
    }
  }, [error]);

  useEffect(() => {
    if (jobError) {
      window.scrollTo(0, document.body.scrollHeight);
    }
  }, [logs, jobError]);

  return (
    <HorizontalForm onSubmit={form.handleSubmit}>
      {/* General info */}
      <FormSection
        title="General"
        description="The name is used to identify the template in URLs and the API."
      >
        <FormFields>
          {"starterTemplate" in props && (
            <SelectedTemplate template={props.starterTemplate} />
          )}
          {"copiedTemplate" in props && (
            <SelectedTemplate template={props.copiedTemplate} />
          )}
          {"upload" in props && (
            <TemplateUpload
              {...props.upload}
              onUpload={async (file) => {
                await fillNameAndDisplayWithFilename(file.name, form);
                props.upload.onUpload(file);
              }}
            />
          )}

          <TextField
            {...getFieldHelpers("name")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            required
            label="Name"
          />
        </FormFields>
      </FormSection>

      {/* Display info  */}
      <FormSection
        title="Display"
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
            disabled={isSubmitting}
            rows={5}
            multiline
            fullWidth
            label="Description"
          />

          <IconField
            {...getFieldHelpers("icon")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            fullWidth
            onPickEmoji={(value) => form.setFieldValue("icon", value)}
          />
        </FormFields>
      </FormSection>

      {/* Variables */}
      {variables && variables.length > 0 && (
        <FormSection
          title="Variables"
          description="Input variables allow you to customize templates without altering their source code."
        >
          <FormFields>
            {variables.map((variable, index) => (
              <VariableInput
                defaultValue={variable.value}
                variable={variable}
                disabled={isSubmitting}
                key={variable.name}
                onChange={async (value) => {
                  await form.setFieldValue("user_variable_values." + index, {
                    name: variable.name,
                    value,
                  });
                }}
              />
            ))}
          </FormFields>
        </FormSection>
      )}

      {jobError && (
        <Stack>
          <div css={styles.error}>
            <h5 css={styles.errorTitle}>Error during provisioning</h5>
            <p css={styles.errorDescription}>
              Looks like we found an error during the template provisioning. You
              can see the logs bellow.
            </p>

            <code css={styles.errorDetails}>{jobError}</code>
          </div>

          <WorkspaceBuildLogs logs={logs ?? []} />
        </Stack>
      )}

      <FormFooter
        onCancel={onCancel}
        isLoading={isSubmitting}
        submitLabel={jobError ? "Retry" : "Create template"}
      />
    </HorizontalForm>
  );
};

const fillNameAndDisplayWithFilename = async (
  filename: string,
  form: ReturnType<typeof useFormik<CreateTemplateData>>,
) => {
  const [name, _extension] = filename.split(".");
  await Promise.all([
    form.setFieldValue(
      "name",
      // Camel case will remove special chars and spaces
      camelCase(name).toLowerCase(),
    ),
    form.setFieldValue("display_name", capitalize(name)),
  ]);
};

const styles = {
  ttlFields: {
    width: "100%",
  },

  optionText: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.primary,
  }),

  optionHelperText: (theme) => ({
    fontSize: 12,
    color: theme.palette.text.secondary,
  }),

  error: (theme) => ({
    padding: 24,
    borderRadius: 8,
    background: theme.palette.background.paper,
    border: `1px solid ${theme.palette.error.main}`,
  }),

  errorTitle: {
    fontSize: 16,
    margin: 0,
  },

  errorDescription: (theme) => ({
    margin: 0,
    color: theme.palette.text.secondary,
    marginTop: 4,
  }),

  errorDetails: (theme) => ({
    display: "block",
    marginTop: 8,
    color: theme.palette.error.light,
    fontSize: 16,
  }),
} satisfies Record<string, Interpolation<Theme>>;
