import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"
import { useTranslation } from "react-i18next"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"

export interface TemplateSettingsPageViewProps {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  errors?: {
    getTemplateError?: unknown
    saveTemplateSettingsError?: unknown
  }
  initialTouched?: ComponentProps<typeof TemplateSettingsForm>["initialTouched"]
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const classes = useStyles()
  const isLoading = !template && !errors.getTemplateError
  const { t } = useTranslation("templateSettingsPage")

  return (
    <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
      {Boolean(errors.getTemplateError) && (
        <Stack className={classes.errorContainer}>
          <AlertBanner severity="error" error={errors.getTemplateError} />
        </Stack>
      )}
      {isLoading && <Loader />}
      {template && (
        <>
          <TemplateSettingsForm
            initialTouched={initialTouched}
            isSubmitting={isSubmitting}
            template={template}
            onSubmit={onSubmit}
            onCancel={onCancel}
            error={errors.saveTemplateSettingsError}
          />
        </>
      )}
    </FullPageHorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  errorContainer: {
    marginBottom: theme.spacing(2),
  },
}))
