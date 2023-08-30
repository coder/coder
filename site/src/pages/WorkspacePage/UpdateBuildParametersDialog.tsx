import { makeStyles } from "@mui/styles"
import Dialog from "@mui/material/Dialog"
import DialogContent from "@mui/material/DialogContent"
import DialogContentText from "@mui/material/DialogContentText"
import DialogTitle from "@mui/material/DialogTitle"
import { DialogProps } from "components/Dialogs/Dialog"
import { FC } from "react"
import { getFormHelpers } from "utils/formUtils"
import { FormFields, VerticalForm } from "components/Form/Form"
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import { useFormik } from "formik"
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters"
import * as Yup from "yup"
import DialogActions from "@mui/material/DialogActions"
import Button from "@mui/material/Button"
import { useTranslation } from "react-i18next"

export type UpdateBuildParametersDialogProps = DialogProps & {
  onClose: () => void
  onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void
  missedParameters: TemplateVersionParameter[]
}

export const UpdateBuildParametersDialog: FC<
  UpdateBuildParametersDialogProps
> = ({ missedParameters, onUpdate, ...dialogProps }) => {
  const styles = useStyles()
  const form = useFormik({
    initialValues: {
      rich_parameter_values: getInitialRichParameterValues(missedParameters),
    },
    validationSchema: Yup.object({
      rich_parameter_values: useValidationSchemaForRichParameters(
        "createWorkspacePage",
        missedParameters,
      ),
    }),
    onSubmit: (values) => {
      onUpdate(values.rich_parameter_values)
    },
    enableReinitialize: true,
  })
  const getFieldHelpers = getFormHelpers(form)
  const { t } = useTranslation("workspacePage")

  return (
    <Dialog
      {...dialogProps}
      scroll="body"
      aria-labelledby="update-build-parameters-title"
      maxWidth="xs"
      data-testid="dialog"
    >
      <DialogTitle
        id="update-build-parameters-title"
        classes={{ root: styles.title }}
      >
        Workspace parameters
      </DialogTitle>
      <DialogContent className={styles.content}>
        <DialogContentText className={styles.info}>
          {t("askParametersDialog.message")}
        </DialogContentText>
        <VerticalForm
          className={styles.form}
          onSubmit={form.handleSubmit}
          id="updateParameters"
        >
          {missedParameters && (
            <FormFields>
              {missedParameters.map((parameter, index) => {
                return (
                  <RichParameterInput
                    {...getFieldHelpers(
                      "rich_parameter_values[" + index + "].value",
                    )}
                    key={parameter.name}
                    parameter={parameter}
                    onChange={async (value) => {
                      await form.setFieldValue(
                        "rich_parameter_values." + index,
                        {
                          name: parameter.name,
                          value: value,
                        },
                      )
                    }}
                  />
                )
              })}
            </FormFields>
          )}
        </VerticalForm>
      </DialogContent>
      <DialogActions disableSpacing className={styles.dialogActions}>
        <Button fullWidth type="button" onClick={dialogProps.onClose}>
          Cancel
        </Button>
        <Button
          color="primary"
          fullWidth
          type="submit"
          form="updateParameters"
          data-testid="form-submit"
        >
          Update
        </Button>
      </DialogActions>
    </Dialog>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    padding: theme.spacing(3, 5),

    "& h2": {
      fontSize: theme.spacing(2.5),
      fontWeight: 400,
    },
  },

  content: {
    padding: theme.spacing(0, 5, 0, 5),
  },

  info: {
    margin: 0,
  },

  form: {
    paddingTop: theme.spacing(4),
  },

  infoTitle: {
    fontSize: theme.spacing(2),
    fontWeight: 600,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
  },

  warningIcon: {
    color: theme.palette.warning.light,
    fontSize: theme.spacing(1.5),
  },

  formFooter: {
    flexDirection: "column",
  },

  dialogActions: {
    padding: theme.spacing(5),
    flexDirection: "column",
    gap: theme.spacing(1),
  },
}))
