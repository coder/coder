import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import MenuItem from "@mui/material/MenuItem";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import { type FormikTouched, useFormik } from "formik";
import { type ChangeEvent, type FC, useState, useEffect } from "react";
import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { DurationField } from "components/DurationField/DurationField";
import {
  FormSection,
  HorizontalForm,
  FormFooter,
  FormFields,
} from "components/Form/Form";
import { Stack } from "components/Stack/Stack";
import {
  StackLabel,
  StackLabelHelperText,
} from "components/StackLabel/StackLabel";
import { getFormHelpers } from "utils/formUtils";
import {
  calculateAutostopRequirementDaysValue,
  type TemplateAutostartRequirementDaysValue,
} from "utils/schedule";
import {
  AutostopRequirementDaysHelperText,
  AutostopRequirementWeeksHelperText,
  convertAutostopRequirementDaysValue,
} from "./AutostopRequirementHelperText";
import {
  getValidationSchema,
  type TemplateScheduleFormValues,
} from "./formHelpers";
import { ScheduleDialog } from "./ScheduleDialog";
import { TemplateScheduleAutostart } from "./TemplateScheduleAutostart";
import {
  ActivityBumpHelperText,
  DefaultTTLHelperText,
  DormancyAutoDeletionTTLHelperText,
  DormancyTTLHelperText,
  FailureTTLHelperText,
} from "./TTLHelperText";
import {
  useWorkspacesToGoDormant,
  useWorkspacesToBeDeleted,
} from "./useWorkspacesToBeDeleted";

const MS_HOUR_CONVERSION = 3600000;
const MS_DAY_CONVERSION = 86400000;
const FAILURE_CLEANUP_DEFAULT = 7 * MS_DAY_CONVERSION;
const INACTIVITY_CLEANUP_DEFAULT = 180 * MS_DAY_CONVERSION;
const DORMANT_AUTODELETION_DEFAULT = 30 * MS_DAY_CONVERSION;
/**
 * The default form field space is 4 but since this form is quite heavy I think
 * increase the space can make it feels lighter.
 */
const FORM_FIELDS_SPACING = 8;
const DORMANT_FIELDSET_SPACING = 4;

export interface TemplateScheduleForm {
  template: Template;
  onSubmit: (data: UpdateTemplateMeta) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  error?: unknown;
  allowAdvancedScheduling: boolean;
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>;
}

export const TemplateScheduleForm: FC<TemplateScheduleForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  allowAdvancedScheduling,
  isSubmitting,
  initialTouched,
}) => {
  const validationSchema = getValidationSchema();
  const form = useFormik<TemplateScheduleFormValues>({
    initialValues: {
      // on display, convert from ms => hours
      default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
      activity_bump_ms: template.activity_bump_ms / MS_HOUR_CONVERSION,
      failure_ttl_ms: template.failure_ttl_ms,
      time_til_dormant_ms: template.time_til_dormant_ms,
      time_til_dormant_autodelete_ms: template.time_til_dormant_autodelete_ms,
      autostop_requirement_days_of_week: allowAdvancedScheduling
        ? convertAutostopRequirementDaysValue(
            template.autostop_requirement.days_of_week,
          )
        : "off",
      autostop_requirement_weeks: allowAdvancedScheduling
        ? template.autostop_requirement.weeks > 0
          ? template.autostop_requirement.weeks
          : 1
        : 1,
      autostart_requirement_days_of_week: template.autostart_requirement
        .days_of_week as TemplateAutostartRequirementDaysValue[],

      allow_user_autostart: template.allow_user_autostart,
      allow_user_autostop: template.allow_user_autostop,
      failure_cleanup_enabled:
        allowAdvancedScheduling && Boolean(template.failure_ttl_ms),
      inactivity_cleanup_enabled:
        allowAdvancedScheduling && Boolean(template.time_til_dormant_ms),
      dormant_autodeletion_cleanup_enabled:
        allowAdvancedScheduling &&
        Boolean(template.time_til_dormant_autodelete_ms),
      update_workspace_last_used_at: false,
      update_workspace_dormant_at: false,
      require_active_version: false,
      disable_everyone_group_access: false,
    },
    validationSchema,
    onSubmit: () => {
      const dormancyChanged =
        form.initialValues.time_til_dormant_ms !==
        form.values.time_til_dormant_ms;
      const deletionChanged =
        form.initialValues.time_til_dormant_autodelete_ms !==
        form.values.time_til_dormant_autodelete_ms;

      const dormancyScheduleChanged =
        form.values.inactivity_cleanup_enabled &&
        dormancyChanged &&
        workspacesToDormancyInWeek &&
        workspacesToDormancyInWeek.length > 0;

      const deletionScheduleChanged =
        form.values.inactivity_cleanup_enabled &&
        deletionChanged &&
        workspacesToBeDeletedInWeek &&
        workspacesToBeDeletedInWeek.length > 0;

      if (dormancyScheduleChanged || deletionScheduleChanged) {
        setIsScheduleDialogOpen(true);
      } else {
        submitValues();
      }
    },
    initialTouched,
    enableReinitialize: true,
  });

  const getFieldHelpers = getFormHelpers<TemplateScheduleFormValues>(
    form,
    error,
  );

  const now = new Date();
  const weekFromNow = new Date(now);
  weekFromNow.setDate(now.getDate() + 7);

  const workspacesToDormancyNow = useWorkspacesToGoDormant(
    template,
    form.values,
    now,
  );

  const workspacesToDormancyInWeek = useWorkspacesToGoDormant(
    template,
    form.values,
    weekFromNow,
  );

  const workspacesToBeDeletedNow = useWorkspacesToBeDeleted(
    template,
    form.values,
    now,
  );

  const workspacesToBeDeletedInWeek = useWorkspacesToBeDeleted(
    template,
    form.values,
    weekFromNow,
  );

  const showScheduleDialog =
    workspacesToDormancyNow &&
    workspacesToBeDeletedNow &&
    workspacesToDormancyInWeek &&
    workspacesToBeDeletedInWeek &&
    (workspacesToDormancyInWeek.length > 0 ||
      workspacesToBeDeletedInWeek.length > 0);

  const [isScheduleDialogOpen, setIsScheduleDialogOpen] =
    useState<boolean>(false);

  const submitValues = () => {
    const autostop_requirement_weeks = ["saturday", "sunday"].includes(
      form.values.autostop_requirement_days_of_week,
    )
      ? form.values.autostop_requirement_weeks
      : 1;

    // on submit, convert from hours => ms
    onSubmit({
      default_ttl_ms: form.values.default_ttl_ms
        ? form.values.default_ttl_ms * MS_HOUR_CONVERSION
        : undefined,
      activity_bump_ms: form.values.activity_bump_ms
        ? form.values.activity_bump_ms * MS_HOUR_CONVERSION
        : undefined,
      failure_ttl_ms: form.values.failure_ttl_ms,
      time_til_dormant_ms: form.values.time_til_dormant_ms,
      time_til_dormant_autodelete_ms:
        form.values.time_til_dormant_autodelete_ms,
      autostop_requirement: {
        days_of_week: calculateAutostopRequirementDaysValue(
          form.values.autostop_requirement_days_of_week,
        ),
        weeks: autostop_requirement_weeks,
      },
      autostart_requirement: {
        days_of_week: form.values.autostart_requirement_days_of_week,
      },
      allow_user_autostart: form.values.allow_user_autostart,
      allow_user_autostop: form.values.allow_user_autostop,
      update_workspace_last_used_at: form.values.update_workspace_last_used_at,
      update_workspace_dormant_at: form.values.update_workspace_dormant_at,
      disable_everyone_group_access: false,
    });
  };

  // Set autostop_requirement weeks to 1 when days_of_week is set to "off" or
  // "daily". Technically you can set weeks to a different value in the backend
  // and it will work, but this is a UX decision so users don't set days=daily
  // and weeks=2 and get confused when workspaces only restart daily during
  // every second week.
  //
  // We want to set the value to 1 when the user selects "off" or "daily"
  // because the input gets disabled so they can't change it to 1 themselves.
  const { values: currentValues, setValues } = form;
  useEffect(() => {
    if (
      !["saturday", "sunday"].includes(
        currentValues.autostop_requirement_days_of_week,
      ) &&
      currentValues.autostop_requirement_weeks !== 1
    ) {
      // This is async but we don't really need to await the value.
      void setValues({
        ...currentValues,
        autostop_requirement_weeks: 1,
      });
    }
  }, [currentValues, setValues]);

  const handleToggleFailureCleanup = async (e: ChangeEvent) => {
    form.handleChange(e);
    if (!form.values.failure_cleanup_enabled) {
      // fill failure_ttl_ms with defaults
      await form.setValues({
        ...form.values,
        failure_cleanup_enabled: true,
        failure_ttl_ms: FAILURE_CLEANUP_DEFAULT,
      });
    } else {
      // clear failure_ttl_ms
      await form.setValues({
        ...form.values,
        failure_cleanup_enabled: false,
        failure_ttl_ms: 0,
      });
    }
  };

  const handleToggleInactivityCleanup = async (e: ChangeEvent) => {
    form.handleChange(e);
    if (!form.values.inactivity_cleanup_enabled) {
      // fill time_til_dormant_ms with defaults
      await form.setValues({
        ...form.values,
        inactivity_cleanup_enabled: true,
        time_til_dormant_ms: INACTIVITY_CLEANUP_DEFAULT,
      });
    } else {
      // clear time_til_dormant_ms
      await form.setValues({
        ...form.values,
        inactivity_cleanup_enabled: false,
        time_til_dormant_ms: 0,
      });
    }
  };

  const handleToggleDormantAutoDeletion = async (e: ChangeEvent) => {
    form.handleChange(e);
    if (!form.values.dormant_autodeletion_cleanup_enabled) {
      // fill failure_ttl_ms with defaults
      await form.setValues({
        ...form.values,
        dormant_autodeletion_cleanup_enabled: true,
        time_til_dormant_autodelete_ms: DORMANT_AUTODELETION_DEFAULT,
      });
    } else {
      // clear failure_ttl_ms
      await form.setValues({
        ...form.values,
        dormant_autodeletion_cleanup_enabled: false,
        time_til_dormant_autodelete_ms: 0,
      });
    }
  };

  return (
    <HorizontalForm
      onSubmit={form.handleSubmit}
      aria-label="Template settings form"
    >
      <FormSection
        title="Autostop"
        description="Define when workspaces created from this template are stopped."
      >
        <FormFields spacing={FORM_FIELDS_SPACING}>
          <TextField
            {...getFieldHelpers("default_ttl_ms", {
              helperText: (
                <DefaultTTLHelperText ttl={form.values.default_ttl_ms} />
              ),
            })}
            disabled={isSubmitting}
            fullWidth
            inputProps={{ min: 0, step: 1 }}
            label="Default autostop (hours)"
            type="number"
          />

          <TextField
            {...getFieldHelpers("activity_bump_ms", {
              helperText: (
                <ActivityBumpHelperText bump={form.values.activity_bump_ms} />
              ),
            })}
            disabled={isSubmitting}
            fullWidth
            inputProps={{ min: 0, step: 1 }}
            label="Activity bump (hours)"
            type="number"
          />

          <Stack direction="row" css={styles.ttlFields}>
            <TextField
              {...getFieldHelpers("autostop_requirement_days_of_week", {
                helperText: (
                  <AutostopRequirementDaysHelperText
                    days={form.values.autostop_requirement_days_of_week}
                  />
                ),
              })}
              disabled={isSubmitting}
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
              {...getFieldHelpers("autostop_requirement_weeks", {
                helperText: (
                  <AutostopRequirementWeeksHelperText
                    days={form.values.autostop_requirement_days_of_week}
                    weeks={form.values.autostop_requirement_weeks}
                  />
                ),
              })}
              disabled={
                isSubmitting ||
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

          <FormControlLabel
            control={
              <Checkbox
                id="allow-user-autostop"
                size="small"
                disabled={isSubmitting || !allowAdvancedScheduling}
                onChange={async (_, checked) => {
                  await form.setFieldValue("allow_user_autostop", checked);
                }}
                name="allow_user_autostop"
                checked={form.values.allow_user_autostop}
              />
            }
            label={
              <StackLabel>
                Allow users to customize autostop duration for workspaces.
                <StackLabelHelperText>
                  By default, workspaces will inherit the Autostop timer from
                  this template. Enabling this option allows users to set custom
                  Autostop timers on their workspaces or turn off the timer.
                </StackLabelHelperText>
              </StackLabel>
            }
          />
        </FormFields>
      </FormSection>

      <FormSection
        title="Autostart"
        description="Allow users to set custom autostart and autostop scheduling options for workspaces created from this template."
      >
        <Stack>
          <FormControlLabel
            control={
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
            }
            label={
              <StackLabel>
                Allow users to automatically start workspaces on a schedule.
              </StackLabel>
            }
          />

          {allowAdvancedScheduling && (
            <TemplateScheduleAutostart
              enabled={Boolean(form.values.allow_user_autostart)}
              value={form.values.autostart_requirement_days_of_week}
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
        </Stack>
      </FormSection>

      {allowAdvancedScheduling && (
        <>
          <FormSection
            title="Dormancy"
            description="When enabled, Coder will mark workspaces as dormant after a period of time with no connections. Dormant workspaces can be auto-deleted (see below) or manually reviewed by the workspace owner or admins."
          >
            <FormFields spacing={FORM_FIELDS_SPACING}>
              <Stack spacing={DORMANT_FIELDSET_SPACING}>
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      name="dormancyThreshold"
                      checked={form.values.inactivity_cleanup_enabled}
                      onChange={handleToggleInactivityCleanup}
                    />
                  }
                  label={<StackLabel>Enable Dormancy Threshold</StackLabel>}
                />

                <DurationField
                  {...getFieldHelpers("time_til_dormant_ms", {
                    helperText: (
                      <DormancyTTLHelperText
                        ttl={form.values.time_til_dormant_ms}
                      />
                    ),
                  })}
                  label="Time until dormant"
                  valueMs={form.values.time_til_dormant_ms ?? 0}
                  onChange={(v) => form.setFieldValue("time_til_dormant_ms", v)}
                  disabled={
                    isSubmitting || !form.values.inactivity_cleanup_enabled
                  }
                />
              </Stack>

              <Stack spacing={DORMANT_FIELDSET_SPACING}>
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      name="dormancyAutoDeletion"
                      checked={form.values.dormant_autodeletion_cleanup_enabled}
                      onChange={handleToggleDormantAutoDeletion}
                    />
                  }
                  label={
                    <StackLabel>
                      Enable Dormancy Auto-Deletion
                      <StackLabelHelperText>
                        When enabled, Coder will permanently delete dormant
                        workspaces after a period of time.{" "}
                        <strong>
                          Once a workspace is deleted it cannot be recovered.
                        </strong>
                      </StackLabelHelperText>
                    </StackLabel>
                  }
                />
                <DurationField
                  {...getFieldHelpers("time_til_dormant_autodelete_ms", {
                    helperText: (
                      <DormancyAutoDeletionTTLHelperText
                        ttl={form.values.time_til_dormant_autodelete_ms}
                      />
                    ),
                  })}
                  label="Time until deletion"
                  valueMs={form.values.time_til_dormant_autodelete_ms ?? 0}
                  onChange={(v) =>
                    form.setFieldValue("time_til_dormant_autodelete_ms", v)
                  }
                  disabled={
                    isSubmitting ||
                    !form.values.dormant_autodeletion_cleanup_enabled
                  }
                />
              </Stack>

              <Stack spacing={DORMANT_FIELDSET_SPACING}>
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      name="failureCleanupEnabled"
                      checked={form.values.failure_cleanup_enabled}
                      onChange={handleToggleFailureCleanup}
                    />
                  }
                  label={
                    <StackLabel>
                      Enable Failure Cleanup
                      <StackLabelHelperText>
                        When enabled, Coder will attempt to stop workspaces that
                        are in a failed state after a period of time.
                      </StackLabelHelperText>
                    </StackLabel>
                  }
                />
                <DurationField
                  {...getFieldHelpers("failure_ttl_ms", {
                    helperText: (
                      <FailureTTLHelperText ttl={form.values.failure_ttl_ms} />
                    ),
                  })}
                  label="Time until cleanup"
                  valueMs={form.values.failure_ttl_ms ?? 0}
                  onChange={(v) => form.setFieldValue("failure_ttl_ms", v)}
                  disabled={
                    isSubmitting || !form.values.failure_cleanup_enabled
                  }
                />
              </Stack>
            </FormFields>
          </FormSection>
        </>
      )}
      {showScheduleDialog && (
        <ScheduleDialog
          onConfirm={() => {
            submitValues();
            setIsScheduleDialogOpen(false);
            // These fields are request-scoped so they should be reset
            // after every submission.
            form
              .setFieldValue("update_workspace_dormant_at", false)
              .catch((error) => {
                throw error;
              });
            form
              .setFieldValue("update_workspace_last_used_at", false)
              .catch((error) => {
                throw error;
              });
          }}
          inactiveWorkspacesToGoDormant={workspacesToDormancyNow.length}
          inactiveWorkspacesToGoDormantInWeek={
            workspacesToDormancyInWeek.length - workspacesToDormancyNow.length
          }
          dormantWorkspacesToBeDeleted={workspacesToBeDeletedNow.length}
          dormantWorkspacesToBeDeletedInWeek={
            workspacesToBeDeletedInWeek.length - workspacesToBeDeletedNow.length
          }
          open={isScheduleDialogOpen}
          onClose={() => {
            setIsScheduleDialogOpen(false);
          }}
          title="Workspace Scheduling"
          updateDormantWorkspaces={(update: boolean) =>
            form.setFieldValue("update_workspace_dormant_at", update)
          }
          updateInactiveWorkspaces={(update: boolean) =>
            form.setFieldValue("update_workspace_last_used_at", update)
          }
          dormantValueChanged={
            form.initialValues.time_til_dormant_ms !==
            form.values.time_til_dormant_ms
          }
          deletionValueChanged={
            form.initialValues.time_til_dormant_autodelete_ms !==
            form.values.time_til_dormant_autodelete_ms
          }
        />
      )}

      <FormFooter
        onCancel={onCancel}
        isLoading={isSubmitting}
        submitDisabled={!form.isValid || !form.dirty}
      />
    </HorizontalForm>
  );
};

const styles = {
  ttlFields: {
    width: "100%",
  },
  dayButtons: {
    borderRadius: "0px",
  },
};
