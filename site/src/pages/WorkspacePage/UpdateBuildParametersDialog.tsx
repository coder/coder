import { css } from "@emotion/css";
import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import type { DialogProps } from "components/Dialogs/Dialog";
import { FormFields, VerticalForm } from "components/Form/Form";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { getFormHelpers } from "utils/formUtils";
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";

export type UpdateBuildParametersDialogProps = DialogProps & {
  onClose: () => void;
  onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void;
  missedParameters: TemplateVersionParameter[];
};

export const UpdateBuildParametersDialog: FC<
  UpdateBuildParametersDialogProps
> = ({ missedParameters, onUpdate, ...dialogProps }) => {
  const form = useFormik({
    initialValues: {
      rich_parameter_values: getInitialRichParameterValues(missedParameters),
    },
    validationSchema: Yup.object({
      rich_parameter_values:
        useValidationSchemaForRichParameters(missedParameters),
    }),
    onSubmit: (values) => {
      onUpdate(values.rich_parameter_values);
    },
    enableReinitialize: true,
  });
  const getFieldHelpers = getFormHelpers(form);

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
        classes={{ root: classNames.root }}
      >
        Workspace parameters
      </DialogTitle>
      <DialogContent css={styles.content}>
        <DialogContentText css={{ margin: 0 }}>
          This template has new parameters that must be configured to complete
          the update
        </DialogContentText>
        <VerticalForm
          css={styles.form}
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
                      );
                    }}
                  />
                );
              })}
            </FormFields>
          )}
        </VerticalForm>
      </DialogContent>
      <DialogActions disableSpacing css={styles.dialogActions}>
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
  );
};

const classNames = {
  root: css`
    padding: 24px 40px;

    & h2 {
      font-size: 20px;
      font-weight: 400;
    }
  `,
};

const styles = {
  content: {
    padding: "0 40px",
  },

  form: {
    paddingTop: 32,
  },

  dialogActions: {
    padding: 40,
    flexDirection: "column",
    gap: 8,
  },
} satisfies Record<string, Interpolation<Theme>>;
