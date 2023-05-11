import { DialogProps } from "components/Dialogs/Dialog"
import { FC } from "react"
import { getFormHelpers } from "utils/formUtils"
import { FormFields } from "components/Form/Form"
import { useFormik } from "formik"
import * as Yup from "yup"
import { PublishVersionData } from "pages/TemplateVersionPage/TemplateVersionEditorPage/types"
import TextField from "@mui/material/TextField"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import Checkbox from "@mui/material/Checkbox"
import FormControlLabel from "@mui/material/FormControlLabel"
import { Stack } from "components/Stack/Stack"

export type PublishTemplateVersionDialogProps = DialogProps & {
  defaultName: string
  isPublishing: boolean
  publishingError?: unknown
  onClose: () => void
  onConfirm: (data: PublishVersionData) => void
}

export const PublishTemplateVersionDialog: FC<
  PublishTemplateVersionDialogProps
> = ({
  onConfirm,
  isPublishing,
  onClose,
  defaultName,
  publishingError,
  ...dialogProps
}) => {
  const form = useFormik({
    initialValues: {
      name: defaultName,
      isActiveVersion: false,
    },
    validationSchema: Yup.object({
      name: Yup.string().required(),
      isActiveVersion: Yup.boolean(),
    }),
    onSubmit: onConfirm,
  })
  const getFieldHelpers = getFormHelpers(form, publishingError)
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
                />
              }
            />
          </FormFields>
        </Stack>
      }
    />
  )
}
