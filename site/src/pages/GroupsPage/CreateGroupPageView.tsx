import TextField from "@mui/material/TextField";
import { CreateGroupRequest } from "api/typesGenerated";
import { FormFooter } from "components/FormFooter/FormFooter";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { IconField } from "components/IconField/IconField";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { FC } from "react";
import { useNavigate } from "react-router-dom";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
  name: Yup.string().required().label("Name"),
});

export type CreateGroupPageViewProps = {
  onSubmit: (data: CreateGroupRequest) => void;
  formErrors?: unknown;
  isLoading: boolean;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
  onSubmit,
  formErrors,
  isLoading,
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
  });
  const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, formErrors);
  const onCancel = () => navigate("/groups");

  return (
    <Margins>
      <FullPageForm title="Create group">
        <form onSubmit={form.handleSubmit}>
          <Stack spacing={2.5}>
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
