import TextField from "@mui/material/TextField";
import { CreateGroupRequest } from "api/typesGenerated";
import { FormFooter } from "components/FormFooter/FormFooter";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { FC } from "react";
import { useNavigate } from "react-router-dom";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
  name: nameValidator("Name"),
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
              onChange={onChangeTrimmed(form)}
              autoComplete="name"
              autoFocus
              fullWidth
              label="Name"
            />
            <TextField
              {...getFieldHelpers(
                "display_name",
                "Optional: keep empty to default to the name.",
              )}
              onChange={onChangeTrimmed(form)}
              autoComplete="display_name"
              autoFocus
              fullWidth
              label="Display Name"
            />
            <TextField
              {...getFieldHelpers("avatar_url")}
              onChange={onChangeTrimmed(form)}
              autoComplete="avatar url"
              fullWidth
              label="Avatar URL"
            />
          </Stack>
          <FormFooter onCancel={onCancel} isLoading={isLoading} />
        </form>
      </FullPageForm>
    </Margins>
  );
};
export default CreateGroupPageView;
