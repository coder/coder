import { Template, TemplateVersionVariable, UpdateTemplateMeta } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateVariablesForm } from "./TemplateVariablesForm"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"
import { useTranslation } from "react-i18next"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"

export interface TemplateVariablesPageViewProps {
  template?: Template
  templateVariables?: TemplateVersionVariable[]
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  errors?: {
    getTemplateError?: unknown
    getTemplateVariablesError?: unknown
  }
  initialTouched?: ComponentProps<typeof TemplateVariablesForm>["initialTouched"]
}

export const TemplateVariablesPageView: FC<TemplateVariablesPageViewProps> = ({
  template,
  templateVariables,
  onCancel,
  onSubmit,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const classes = useStyles()
  const isLoading = !template && !errors.getTemplateError && !errors.getTemplateVariablesError
  const { t } = useTranslation("TemplateVariablesPage")

  return (
    <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
      {Boolean(errors.getTemplateError) && (
        <Stack className={classes.errorContainer}>
          <AlertBanner severity="error" error={errors.getTemplateError} />
        </Stack>
      )}
      {isLoading && <Loader />}
      {template && templateVariables && (
        <>
          <TemplateVariablesForm
            initialTouched={initialTouched}
            isSubmitting={isSubmitting}
            template={template}
            templateVariables={templateVariables}
            onSubmit={onSubmit}
            onCancel={onCancel}
            // TODO error={errors.saveTemplateVariablesError}
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
