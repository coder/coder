import TextField from "@mui/material/TextField";
import { type FormikTouched, useFormik } from "formik";
import { type FC } from "react";
import { useNavigate } from "react-router-dom";
import * as Yup from "yup";
import type { CreateGroupRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { FormFooter } from "components/FormFooter/FormFooter";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { IconField } from "components/IconField/IconField";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import { isApiValidationError } from "api/errors";

const validationSchema = Yup.object({
  name: Yup.string().required().label("Name"),
});

export type CreateGroupPageViewProps = {
  onSubmit: (data: CreateGroupRequest) => void;
  error?: unknown;
  isLoading: boolean;
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<CreateGroupRequest>;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
  onSubmit,
  error,
  isLoading,
  initialTouched,
}) => {
  const navigate = useNavigate();
  const form = useFormik<CreateGroupRequest>({
    initialValues: {
      name: "",
      display_name: "",
      avatar_url: "",
      quota_allowance: 0,
    },
    validationSchema,
    onSubmit,
    initialTouched,
  });
  const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, error);
  const onCancel = () => navigate("/groups");

  return (
    <Margins>
      <FullPageForm title="Create group">
        <form onSubmit={form.handleSubmit}>
          <Stack spacing={2.5}>
            {Boolean(error) && !isApiValidationError(error) && (
              <ErrorAlert error={error} />
            )}

            <TextField
              {...getFieldHelpers("name")}
              autoFocus
              fullWidth
              label="Name"
            />
            <TextField
              {...getFieldHelpers("display_name", {
                helperText: "Optional: keep empty to default to the name.",
              })}
              fullWidth
              label="Display Name"
            />
            <IconField
              {...getFieldHelpers("avatar_url")}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label="Avatar URL"
              onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
            />
          </Stack>
          <FormFooter onCancel={onCancel} isLoading={isLoading} />
        </form>
      </FullPageForm>
    </Margins>
  );
};
export default CreateGroupPageView;
