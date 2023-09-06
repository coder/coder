import TextField from "@mui/material/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import {
  getFormHelpers,
  nameValidator,
  templateDisplayNameValidator,
  onChangeTrimmed,
  iconValidator,
} from "utils/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"
import { LazyIconField } from "components/IconField/LazyIconField"
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
import Checkbox from "@mui/material/Checkbox"
import {
  HelpTooltip,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip"
import { makeStyles } from "@mui/styles"

const MAX_DESCRIPTION_CHAR_LIMIT = 128

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    name: nameValidator(i18next.t("nameLabel", { ns: "templateSettingsPage" })),
    display_name: templateDisplayNameValidator(
      i18next.t("displayNameLabel", {
        ns: "templateSettingsPage",
      }),
    ),
    description: Yup.string().max(
      MAX_DESCRIPTION_CHAR_LIMIT,
      i18next
        .t("descriptionMaxError", { ns: "templateSettingsPage" })
        .toString(),
    ),
    allow_user_cancel_workspace_jobs: Yup.boolean(),
    icon: iconValidator,
  })

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
}) => {
  const validationSchema = getValidationSchema()
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
      },
      validationSchema,
      onSubmit,
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers(form, error)
  const { t } = useTranslation("templateSettingsPage")
  const styles = useStyles()

  return (
    <HorizontalForm
      onSubmit={form.handleSubmit}
      aria-label={t("formAriaLabel").toString()}
    >
      <FormSection
        title={t("generalInfo.title").toString()}
        description={t("generalInfo.description").toString()}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label={t("nameLabel")}
          />
        </FormFields>
      </FormSection>

      <FormSection
        title={t("displayInfo.title").toString()}
        description={t("displayInfo.description").toString()}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("display_name")}
            disabled={isSubmitting}
            fullWidth
            label={t("displayNameLabel")}
          />

          <TextField
            {...getFieldHelpers("description")}
            multiline
            disabled={isSubmitting}
            fullWidth
            label={t("descriptionLabel")}
            rows={2}
          />

          <LazyIconField
            {...getFieldHelpers("icon")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            fullWidth
            label={t("iconLabel")}
            onPickEmoji={(value) => form.setFieldValue("icon", value)}
          />
        </FormFields>
      </FormSection>

      <FormSection
        title={t("operations.title").toString()}
        description={t("operations.description").toString()}
      >
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
                {t("allowUserCancelWorkspaceJobsLabel")}

                <HelpTooltip>
                  <HelpTooltipText>
                    {t("allowUserCancelWorkspaceJobsNotice")}
                  </HelpTooltipText>
                </HelpTooltip>
              </Stack>
              <span className={styles.optionHelperText}>
                {t("allowUsersCancelHelperText")}
              </span>
            </Stack>
          </Stack>
        </label>
      </FormSection>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  optionText: {
    fontSize: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  optionHelperText: {
    fontSize: theme.spacing(1.5),
    color: theme.palette.text.secondary,
  },
}))
