import {
  CreateTemplateVersionRequest,
  TemplateVersion,
  TemplateVersionVariable,
} from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateVariablesForm } from "./TemplateVariablesForm"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"
import { useTranslation } from "react-i18next"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"

export interface TemplateVariablesPageViewProps {
  templateVersion?: TemplateVersion
  templateVariables?: TemplateVersionVariable[]
  onSubmit: (data: CreateTemplateVersionRequest) => void
  onCancel: () => void
  isSubmitting: boolean
  errors?: {
    getTemplateError?: unknown
    getActiveTemplateVersionError?: unknown
    getTemplateVariablesError?: unknown
    updateTemplateError?: unknown
  }
  initialTouched?: ComponentProps<
    typeof TemplateVariablesForm
  >["initialTouched"]
}

export const TemplateVariablesPageView: FC<TemplateVariablesPageViewProps> = ({
  templateVersion,
  templateVariables,
  onCancel,
  onSubmit,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const classes = useStyles()
  const isLoading =
    !templateVersion &&
    !templateVariables &&
    !errors.getTemplateError &&
    !errors.getTemplateVariablesError
  const { t } = useTranslation("templateVariablesPage")

  // TODO stack alert banners
  return (
    <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
      {Boolean(errors.getTemplateError) && (
        <Stack className={classes.errorContainer}>
          <AlertBanner severity="error" error={errors.getTemplateError} />
        </Stack>
      )}
      {isLoading && <Loader />}
      {templateVersion && templateVariables && (
        <TemplateVariablesForm
          initialTouched={initialTouched}
          isSubmitting={isSubmitting}
          templateVersion={templateVersion}
          templateVariables={templateVariables}
          onSubmit={onSubmit}
          onCancel={onCancel}
          error={errors.updateTemplateError}
        />
      )}
    </FullPageHorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  errorContainer: {
    marginBottom: theme.spacing(2),
  },
}))
