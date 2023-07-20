import { UpdateTemplateMeta } from "api/typesGenerated"
import * as Yup from "yup"
import i18next from "i18next"

export interface TemplateScheduleFormValues extends UpdateTemplateMeta {
  failure_cleanup_enabled: boolean
  inactivity_cleanup_enabled: boolean
  locked_cleanup_enabled: boolean
}

const MAX_TTL_DAYS = 30

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    default_ttl_ms: Yup.number()
      .integer()
      .min(
        0,
        i18next
          .t("defaultTTLMinError", { ns: "templateSettingsPage" })
          .toString(),
      )
      .max(
        24 * MAX_TTL_DAYS /* 30 days in hours */,
        i18next
          .t("defaultTTLMaxError", { ns: "templateSettingsPage" })
          .toString(),
      ),
    max_ttl_ms: Yup.number()
      .integer()
      .min(
        0,
        i18next.t("maxTTLMinError", { ns: "templateSettingsPage" }).toString(),
      )
      .max(
        24 * MAX_TTL_DAYS /* 30 days in hours */,
        i18next.t("maxTTLMaxError", { ns: "templateSettingsPage" }).toString(),
      ),
    failure_ttl_ms: Yup.number()
      .min(0, "Failure cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Failure cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues
          if (parent.failure_cleanup_enabled) {
            return Boolean(value)
          } else {
            return true
          }
        },
      ),
    inactivity_ttl_ms: Yup.number()
      .min(0, "Inactivity cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Inactivity cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues
          if (parent.inactivity_cleanup_enabled) {
            return Boolean(value)
          } else {
            return true
          }
        },
      ),
    locked_ttl_ms: Yup.number()
      .min(0, "Locked cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Locked cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues
          if (parent.locked_cleanup_enabled) {
            return Boolean(value)
          } else {
            return true
          }
        },
      ),
    allow_user_autostart: Yup.boolean(),
    allow_user_autostop: Yup.boolean(),
  })
