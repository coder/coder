import * as Yup from "yup";
import type { UpdateTemplateMeta } from "api/typesGenerated";
import type {
  TemplateAutostartRequirementDaysValue,
  TemplateAutostopRequirementDaysValue,
} from "utils/schedule";

export interface TemplateScheduleFormValues
  extends Omit<
    UpdateTemplateMeta,
    "autostop_requirement" | "autostart_requirement"
  > {
  autostart_requirement_days_of_week: TemplateAutostartRequirementDaysValue[];
  autostop_requirement_days_of_week: TemplateAutostopRequirementDaysValue;
  autostop_requirement_weeks: number;
  failure_cleanup_enabled: boolean;
  inactivity_cleanup_enabled: boolean;
  dormant_autodeletion_cleanup_enabled: boolean;
}

const MAX_TTL_DAYS = 30;

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    default_ttl_ms: Yup.number()
      .integer()
      .required()
      .min(0, "Default time until autostop must not be less than 0.")
      .max(
        24 * MAX_TTL_DAYS /* 30 days in hours */,
        "Please enter a limit that is less than or equal to 720 hours (30 days).",
      ),
    activity_bump_ms: Yup.number()
      .integer()
      .required()
      .min(0, "Activity bump must not be less than 0.")
      .max(
        24 * MAX_TTL_DAYS /* 30 days in hours */,
        "Please enter an activity bump duration that is less than or equal to 720 hours (30 days).",
      ),
    failure_ttl_ms: Yup.number()
      .integer()
      .required()
      .min(0, "Failure cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Failure cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues;
          if (parent.failure_cleanup_enabled) {
            return Boolean(value);
          } else {
            return true;
          }
        },
      ),
    time_til_dormant_ms: Yup.number()
      .integer()
      .required()
      .min(0, "Dormancy threshold days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Dormancy threshold days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues;
          if (parent.inactivity_cleanup_enabled) {
            return Boolean(value);
          } else {
            return true;
          }
        },
      ),
    time_til_dormant_autodelete_ms: Yup.number()
      .integer()
      .required()
      .min(0, "Dormancy auto-deletion days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Dormancy auto-deletion days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues;
          if (parent.dormant_autodeletion_cleanup_enabled) {
            return Boolean(value);
          } else {
            return true;
          }
        },
      ),
    allow_user_autostart: Yup.boolean(),
    allow_user_autostop: Yup.boolean(),

    autostop_requirement_days_of_week: Yup.string().required(),
    autostart_requirement_days_of_week: Yup.array().of(Yup.string()).required(),
    autostop_requirement_weeks: Yup.number().required().min(1).max(16),
  });
