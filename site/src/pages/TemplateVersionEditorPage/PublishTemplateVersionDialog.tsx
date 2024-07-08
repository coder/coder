import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { DialogProps } from "components/Dialogs/Dialog";
import { FormFields } from "components/Form/Form";
import { Stack } from "components/Stack/Stack";
import type { PublishVersionData } from "pages/TemplateVersionEditorPage/types";
import { getFormHelpers } from "utils/formUtils";

export const Language = {
  versionNameLabel: "Version name",
  messagePlaceholder: "Write a short message about the changes you made...",
  defaultCheckboxLabel: "Promote to default version",
};

export type PublishTemplateVersionDialogProps = DialogProps & {
  defaultName: string;
  isPublishing: boolean;
  publishingError?: unknown;
  onClose: () => void;
  onConfirm: (data: PublishVersionData) => void;
};

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
      message: "",
      isActiveVersion: true,
    },
    validationSchema: Yup.object({
      name: Yup.string().required(),
      message: Yup.string(),
      isActiveVersion: Yup.boolean(),
    }),
    onSubmit: onConfirm,
  });
  const getFieldHelpers = getFormHelpers(form, publishingError);
  const handleClose = () => {
    form.resetForm();
    onClose();
  };

  return (
    <ConfirmDialog
      {...dialogProps}
      confirmLoading={isPublishing}
      onClose={handleClose}
      onConfirm={async () => {
        await form.submitForm();
      }}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Publish"
      title="Publish new version"
      description={
        <form id="publish-version" onSubmit={form.handleSubmit}>
          <Stack>
            <p>You are about to publish a new version of this template.</p>
            <FormFields>
              <TextField
                {...getFieldHelpers("name")}
                label={Language.versionNameLabel}
                autoFocus
                disabled={isPublishing}
              />

              <TextField
                {...getFieldHelpers("message")}
                label="Message"
                placeholder={Language.messagePlaceholder}
                disabled={isPublishing}
                multiline
                rows={5}
              />

              <FormControlLabel
                label={Language.defaultCheckboxLabel}
                control={
                  <Checkbox
                    size="small"
                    checked={form.values.isActiveVersion}
                    onChange={async (e) => {
                      await form.setFieldValue(
                        "isActiveVersion",
                        e.target.checked,
                      );
                    }}
                    name="isActiveVersion"
                  />
                }
              />
            </FormFields>
          </Stack>
        </form>
      }
    />
  );
};
