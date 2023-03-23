import { DialogProps } from "components/Dialogs/Dialog"
import { FC } from "react"
import { getFormHelpers, nameValidator } from "util/formUtils"
import { FormFields } from "components/Form/Form"
import { useFormik } from "formik"
import * as Yup from "yup"
import { PublishVersionData } from "pages/TemplateVersionPage/TemplateVersionEditorPage/types"
import TextField from "@material-ui/core/TextField"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import Checkbox from "@material-ui/core/Checkbox"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import { Stack } from "components/Stack/Stack"

export type PublishTemplateVersionDialogProps = DialogProps & {
  isPublishing: boolean
  onClose: () => void
  onConfirm: (data: PublishVersionData) => void
}

export const PublishTemplateVersionDialog: FC<
  PublishTemplateVersionDialogProps
> = ({ onConfirm, isPublishing, onClose, ...dialogProps }) => {
  const form = useFormik({
    initialValues: {
      name: "",
      isActiveVersion: false,
    },
    validationSchema: Yup.object({
      name: nameValidator("name").optional(),
      isActiveVersion: Yup.boolean(),
    }),
    onSubmit: onConfirm,
  })
  const getFieldHelpers = getFormHelpers(form)
  const handleClose = () => {
    form.resetForm()
    onClose()
  }

  return (
    <ConfirmDialog
      {...dialogProps}
      confirmLoading={isPublishing}
      onClose={handleClose}
      onConfirm={async () => {
        await form.submitForm()
      }}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Publish"
      title="Publish new version"
      description={
        <Stack>
          <p>You are about to publish a new version of this template.</p>
          <FormFields>
            <TextField
              {...getFieldHelpers("name")}
              label="Version name"
              autoFocus
              disabled={isPublishing}
              InputLabelProps={{
                shrink: true,
              }}
              helperText="If you leave this blank, the version name will be automatically generated."
            />

            <FormControlLabel
              label="Promote to default version"
              control={
                <Checkbox
                  size="small"
                  checked={form.values.isActiveVersion}
                  onChange={async (e) => {
                    await form.setFieldValue(
                      "isActiveVersion",
                      e.target.checked,
                    )
                  }}
                  name="isActiveVersion"
                  color="primary"
                />
              }
            />
          </FormFields>
        </Stack>
      }
    />
  )
}
