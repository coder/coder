import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"
import { Stack } from "components/Stack/Stack"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import { makeStyles } from "@material-ui/core/styles"
import { colors } from "theme/colors"
import Button from "@material-ui/core/Button"
import { useTranslation } from "react-i18next"
import { Navigate } from "react-router-dom"

export interface TemplateSettingsPageViewProps {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  onDelete: () => void
  onConfirmDelete: () => void
  onCancelDelete: () => void
  isConfirmingDelete: boolean
  isDeleting: boolean
  isDeleted: boolean
  isSubmitting: boolean
  errors?: {
    getTemplateError?: unknown
    saveTemplateSettingsError?: unknown
    deleteTemplateError?: unknown
  }
  initialTouched?: ComponentProps<typeof TemplateSettingsForm>["initialTouched"]
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  onDelete,
  onConfirmDelete,
  onCancelDelete,
  isConfirmingDelete,
  isDeleting,
  isDeleted,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const classes = useStyles()
  const isLoading = !template && !errors.getTemplateError
  const { t } = useTranslation("templateSettingsPage")

  if (isDeleted) {
    return <Navigate to="/templates" />
  }

  return (
    <FullPageForm title={t("title")}>
      {Boolean(errors.getTemplateError) && (
        <Stack className={classes.errorContainer}>
          <AlertBanner severity="error" error={errors.getTemplateError} />
        </Stack>
      )}
      {Boolean(errors.deleteTemplateError) && (
        <Stack className={classes.errorContainer}>
          <AlertBanner severity="error" error={errors.deleteTemplateError} />
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
          <Stack className={classes.dangerContainer}>
            <div className={classes.dangerHeader}>
              {t("dangerZone.dangerZoneHeader")}
            </div>

            <Stack className={classes.dangerBorder}>
              <Stack spacing={0}>
                <p className={classes.deleteTemplateHeader}>
                  {t("dangerZone.deleteTemplateHeader")}
                </p>
                <span>{t("dangerZone.deleteTemplateCaption")}</span>
              </Stack>
              <Button
                className={classes.deleteButton}
                onClick={onDelete}
                aria-label={t("dangerZone.deleteCta")}
              >
                {t("dangerZone.deleteCta")}
              </Button>
            </Stack>
          </Stack>

          <DeleteDialog
            isOpen={isConfirmingDelete}
            confirmLoading={isDeleting}
            onConfirm={onConfirmDelete}
            onCancel={onCancelDelete}
            entity="template"
            name={template.name}
          />
        </>
      )}
    </FullPageForm>
  )
}

const useStyles = makeStyles((theme) => ({
  errorContainer: {
    marginBottom: theme.spacing(2),
  },
  dangerContainer: {
    marginTop: theme.spacing(4),
  },
  dangerHeader: {
    fontSize: theme.typography.h5.fontSize,
    color: theme.palette.text.secondary,
  },
  dangerBorder: {
    border: `1px solid ${colors.red[13]}`,
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(2),

    "& p": {
      marginTop: "0px",
    },
  },
  deleteTemplateHeader: {
    fontSize: theme.typography.h6.fontSize,
    fontWeight: "bold",
  },
  deleteButton: {
    color: colors.red[8],
  },
}))
