import { makeStyles } from "@material-ui/core/styles"
import Dialog from "@material-ui/core/Dialog"
import DialogContent from "@material-ui/core/DialogContent"
import DialogContentText from "@material-ui/core/DialogContentText"
import DialogTitle from "@material-ui/core/DialogTitle"
import { DialogProps } from "components/Dialogs/Dialog"
import { FC } from "react"
import { getFormHelpers } from "util/formUtils"
import {
  FormFields,
  FormFooter,
} from "components/HorizontalForm/HorizontalForm"
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import {
  selectInitialRichParametersValues,
  ValidationSchemaForRichParameters,
} from "util/richParameters"
import * as Yup from "yup"

export type UpdateBuildParametersDialogProps = DialogProps & {
  onClose: () => void
  onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void
  parameters?: TemplateVersionParameter[]
}

export const UpdateBuildParametersDialog: FC<
  UpdateBuildParametersDialogProps
> = ({ parameters, onUpdate, ...dialogProps }) => {
  const styles = useStyles()
  const form = useFormik({
    initialValues: {
      rich_parameter_values: selectInitialRichParametersValues(parameters),
    },
    validationSchema: Yup.object({
      rich_parameter_values: ValidationSchemaForRichParameters(
        "createWorkspacePage",
        parameters,
      ),
    }),
    onSubmit: (values) => {
      onUpdate(values.rich_parameter_values)
    },
  })
  const getFieldHelpers = getFormHelpers(form)

  return (
    <Dialog {...dialogProps} aria-labelledby="update-build-parameters-title">
      <DialogTitle
        id="update-build-parameters-title"
        classes={{ root: styles.title }}
      >
        Missing workspace parameters
      </DialogTitle>
      <DialogContent className={styles.content}>
        <DialogContentText className={styles.contentText}>
          It looks like the new version has some mandatory parameters that need
          to be filled in to update the workspace.
        </DialogContentText>
        <form className={styles.form} onSubmit={form.handleSubmit}>
          <Stack spacing={5}>
            <FormFields>
              {parameters &&
                parameters.map((parameter, index) => {
                  return (
                    <RichParameterInput
                      {...getFieldHelpers(
                        "rich_parameter_values[" + index + "].value",
                      )}
                      key={parameter.name}
                      parameter={parameter}
                      initialValue=""
                      index={index}
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
            <FormFooter onCancel={dialogProps.onClose} isLoading={false} />
          </Stack>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    padding: theme.spacing(5, 5, 2, 5),

    "& h2": {
      fontSize: theme.spacing(2.5),
      fontWeight: 400,
    },
  },

  content: {
    padding: theme.spacing(0, 5, 5, 5),
  },

  contentText: {
    fontSize: theme.spacing(2),
    lineHeight: "160%",
  },

  form: {
    marginTop: theme.spacing(4),
  },
}))
