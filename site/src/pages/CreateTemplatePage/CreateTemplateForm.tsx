import { type Interpolation, type Theme } from "@emotion/react";
import Checkbox from "@mui/material/Checkbox";
import TextField from "@mui/material/TextField";
import type {
  ProvisionerJobLog,
  Template,
  TemplateExample,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { TemplateUpload, type TemplateUploadProps } from "./TemplateUpload";
import { useFormik } from "formik";
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate";
import { type FC, useEffect } from "react";
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
  templateDisplayNameValidator,
} from "utils/formUtils";
import * as Yup from "yup";
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
  HelpTooltip,
  HelpTooltipContent,
  HelpTooltipText,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { LazyIconField } from "components/IconField/LazyIconField";
import Link from "@mui/material/Link";
import {
  HorizontalForm,
  FormSection,
  FormFields,
  FormFooter,
} from "components/Form/Form";
import camelCase from "lodash/camelCase";
import capitalize from "lodash/capitalize";
import { VariableInput } from "./VariableInput";
import { docs } from "utils/docs";
import {
  AutostopRequirementDaysHelperText,
  AutostopRequirementWeeksHelperText,
} from "pages/TemplateSettingsPage/TemplateSchedulePage/AutostopRequirementHelperText";
import MenuItem from "@mui/material/MenuItem";
import {
  type TemplateAutostartRequirementDaysValue,
  type TemplateAutostopRequirementDaysValue,
} from "utils/schedule";
import {
  TemplateScheduleAutostart,
  sortedDays,
} from "components/TemplateScheduleAutostart/TemplateScheduleAutostart";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_TTL_DAYS = 30;

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
  default_ttl_hours: Yup.number()
    .integer()
    .min(0, "Default time until autostop must not be less than 0.")
    .max(
      24 * MAX_TTL_DAYS /* 30 days in hours */,
      "Please enter a limit that is less than or equal to 720 hours (30 days).",
    ),
  max_ttl_hours: Yup.number()
    .integer()
    .min(0, "Maximum time until autostop must not be less than 0.")
    .max(
      24 * MAX_TTL_DAYS /* 30 days in hours */,
      "Please enter a limit that is less than or equal to 720 hours (30 days).",
    ),
  autostop_requirement_days_of_week: Yup.string().required(),
  autostop_requirement_weeks: Yup.number().required().min(1).max(16),
  autostart_requirement_days_of_week: Yup.array().of(Yup.string()).required(),
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
  allowDisableEveryoneAccess: boolean;
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
    allowDisableEveryoneAccess,
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

  // Set autostop_requirement weeks to 1 when days_of_week is set to "off" or
  // "daily". Technically you can set weeks to a different value in the backend
  // and it will work, but this is a UX decision so users don't set days=daily
  // and weeks=2 and get confused when workspaces only restart daily during
  // every second week.
  //
  // We want to set the value to 1 when the user selects "off" or "daily"
  // because the input gets disabled so they can't change it to 1 themselves.
  const {
    values: { autostop_requirement_days_of_week },
    setFieldValue,
  } = form;
  useEffect(() => {
    if (!["saturday", "sunday"].includes(autostop_requirement_days_of_week)) {
      // This is async but we don't really need to await the value.
      void setFieldValue("autostop_requirement_weeks", 1);
    }
  }, [autostop_requirement_days_of_week, setFieldValue]);

  const handleToggleUseMaxTTL = async () => {
    const val = !form.values.use_max_ttl;
    if (val) {
      // set max_ttl to 1, set autostop_requirement to empty
      await form.setValues({
        ...form.values,
        use_max_ttl: val,
        max_ttl_hours: 1,
        autostop_requirement_days_of_week: "off",
        autostop_requirement_weeks: 1,
      });
    } else {
      // set max_ttl to 0
      await form.setValues({
        ...form.values,
        use_max_ttl: val,
        max_ttl_hours: 0,
      });
    }
  };

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
            {...getFieldHelpers("description")}
            disabled={isSubmitting}
            rows={5}
            multiline
            fullWidth
            label="Description"
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

      {/* Schedule */}
      <FormSection
        title="Schedule"
        description="Define when workspaces created from this template automatically stop."
      >
        <FormFields>
          <Stack direction="row" css={styles.ttlFields}>
            <TextField
              {...getFieldHelpers(
                "default_ttl_hours",
                <DefaultTTLHelperText ttl={form.values.default_ttl_hours} />,
              )}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label="Default autostop (hours)"
              type="number"
            />
          </Stack>

          <Stack direction="row" css={styles.ttlFields}>
            <TextField
              {...getFieldHelpers(
                "autostop_requirement_days_of_week",
                <AutostopRequirementDaysHelperText
                  days={form.values.autostop_requirement_days_of_week}
                />,
              )}
              disabled={
                isSubmitting ||
                form.values.use_max_ttl ||
                !allowAdvancedScheduling
              }
              fullWidth
              select
              value={form.values.autostop_requirement_days_of_week}
              label="Days with required stop"
            >
              <MenuItem key="off" value="off">
                Off
              </MenuItem>
              <MenuItem key="daily" value="daily">
                Daily
              </MenuItem>
              <MenuItem key="saturday" value="saturday">
                Saturday
              </MenuItem>
              <MenuItem key="sunday" value="sunday">
                Sunday
              </MenuItem>
            </TextField>

            <TextField
              {...getFieldHelpers(
                "autostop_requirement_weeks",
                <AutostopRequirementWeeksHelperText
                  days={form.values.autostop_requirement_days_of_week}
                  weeks={form.values.autostop_requirement_weeks}
                />,
              )}
              disabled={
                isSubmitting ||
                form.values.use_max_ttl ||
                !allowAdvancedScheduling ||
                !["saturday", "sunday"].includes(
                  form.values.autostop_requirement_days_of_week || "",
                )
              }
              fullWidth
              inputProps={{ min: 1, max: 16, step: 1 }}
              label="Weeks between required stops"
              type="number"
            />
          </Stack>

          <Stack direction="column">
            <Stack direction="row" alignItems="center">
              <Checkbox
                id="use_max_ttl"
                size="small"
                disabled={isSubmitting || !allowAdvancedScheduling}
                onChange={handleToggleUseMaxTTL}
                name="use_max_ttl"
                checked={form.values.use_max_ttl}
              />
              <Stack spacing={0.5}>
                <strong>
                  Use a max lifetime instead of a required autostop schedule.
                </strong>
                <span css={styles.optionHelperText}>
                  Use a maximum lifetime for workspaces created from this
                  template instead of an autostop requirement as configured
                  above.
                </span>
              </Stack>
            </Stack>

            <TextField
              {...getFieldHelpers(
                "max_ttl_hours",
                allowAdvancedScheduling ? (
                  <MaxTTLHelperText ttl={form.values.max_ttl_hours} />
                ) : (
                  <>
                    You need an enterprise license to use it.{" "}
                    <Link href={docs("/enterprise")}>Learn more</Link>.
                  </>
                ),
              )}
              disabled={
                isSubmitting ||
                !form.values.use_max_ttl ||
                !allowAdvancedScheduling
              }
              fullWidth
              label="Max lifetime (hours)"
              type="number"
            />
          </Stack>

          <Stack direction="column">
            <Stack direction="row" alignItems="center">
              <Checkbox
                id="allow_user_autostart"
                size="small"
                disabled={isSubmitting || !allowAdvancedScheduling}
                onChange={async () => {
                  await form.setFieldValue(
                    "allow_user_autostart",
                    !form.values.allow_user_autostart,
                  );
                }}
                name="allow_user_autostart"
                checked={form.values.allow_user_autostart}
              />
              <Stack spacing={0.5}>
                <strong>
                  Allow users to automatically start workspaces on a schedule.
                </strong>
              </Stack>
            </Stack>

            {allowAdvancedScheduling && (
              <TemplateScheduleAutostart
                allow_user_autostart={form.values.allow_user_autostart}
                autostart_requirement_days_of_week={
                  form.values.autostart_requirement_days_of_week
                }
                isSubmitting={isSubmitting}
                onChange={async (
                  newDaysOfWeek: TemplateAutostartRequirementDaysValue[],
                ) => {
                  await form.setFieldValue(
                    "autostart_requirement_days_of_week",
                    newDaysOfWeek,
                  );
                }}
              />
            )}

            <Stack direction="row" alignItems="center">
              <Checkbox
                id="allow-user-autostop"
                size="small"
                disabled={isSubmitting || !allowAdvancedScheduling}
                onChange={async () => {
                  await form.setFieldValue(
                    "allow_user_autostop",
                    !form.values.allow_user_autostop,
                  );
                }}
                name="allow-user-autostop"
                checked={form.values.allow_user_autostop}
              />
              <Stack spacing={0.5}>
                <strong>
                  Allow users to customize autostop duration for workspaces.
                </strong>
                <span css={styles.optionHelperText}>
                  Workspaces will always use the default TTL if this is set.
                  Regardless of this setting, workspaces will still stop due to
                  the autostop requirement policy.
                </span>
              </Stack>
            </Stack>
          </Stack>
        </FormFields>
      </FormSection>

      {/* Permissions */}
      <FormSection
        title="Permissions"
        description="Regulate actions allowed on workspaces created from this template."
      >
        <Stack direction="column">
          <FormFields>
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
                    <strong>
                      Allow users to cancel in-progress workspace jobs
                    </strong>

                    <HelpTooltip>
                      <HelpTooltipTrigger size="small" />
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
          </FormFields>
          <FormFields>
            <label htmlFor="allow_everyone_group_access">
              <Stack direction="row" spacing={1}>
                <Checkbox
                  id="allow_everyone_group_access"
                  name="allow_everyone_group_access"
                  disabled={isSubmitting || !allowDisableEveryoneAccess}
                  checked={form.values.allow_everyone_group_access}
                  onChange={form.handleChange}
                />

                <Stack direction="column" spacing={0.5}>
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={0.5}
                    css={styles.optionText}
                  >
                    <strong>Allow everyone to use the template</strong>

                    <HelpTooltip>
                      <HelpTooltipTrigger size="small" />
                      <HelpTooltipContent>
                        <HelpTooltipText>
                          If unchecked, only users with the &apos;template
                          admin&apos; and &apos;owner&apos; role can use this
                          template until the permissions are updated. Navigate
                          to{" "}
                          <strong>
                            Templates <MenuPath /> Select a template{" "}
                            <MenuPath /> Settings <MenuPath />
                            Permissions
                          </strong>{" "}
                          to update permissions.
                        </HelpTooltipText>
                      </HelpTooltipContent>
                    </HelpTooltip>
                  </Stack>
                  <span css={styles.optionHelperText}>
                    This setting requires an enterprise license for the&nbsp;
                    <Link href={docs("/admin/rbac")}>
                      &apos;Template RBAC&apos;
                    </Link>{" "}
                    feature to customize permissions.
                  </span>
                </Stack>
              </Stack>
            </label>
          </FormFields>
        </Stack>
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

const MenuPath = () => {
  return <span aria-label="in">&gt;</span>;
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

const hours = (h: number) => (h === 1 ? "hour" : "hours");

const DefaultTTLHelperText = (props: { ttl?: number }) => {
  const { ttl = 0 } = props;

  // Error will show once field is considered touched
  if (ttl < 0) {
    return null;
  }

  if (ttl === 0) {
    return <span>Workspaces will run until stopped manually.</span>;
  }

  return (
    <span>
      Workspaces will default to stopping after {ttl} {hours(ttl)} without
      activity.
    </span>
  );
};

const MaxTTLHelperText = (props: { ttl?: number }) => {
  const { ttl = 0 } = props;

  // Error will show once field is considered touched
  if (ttl < 0) {
    return null;
  }

  if (ttl === 0) {
    return <span>Workspaces may run indefinitely.</span>;
  }

  return (
    <span>
      Workspaces must stop within {ttl} {hours(ttl)} of starting, regardless of
      any active connections.
    </span>
  );
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
