import type { Interpolation, Theme } from "@emotion/react";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import * as Yup from "yup";
import { isApiValidationError } from "api/errors";
import { RBACResourceActions } from "api/rbacresources_gen";
import type { Role } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { getFormHelpers } from "utils/formUtils";

const validationSchema = Yup.object({
  name: Yup.string().required().label("Name"),
});

export type CreateEditRolePageViewProps = {
  onSubmit: (data: Role) => void;
  error?: unknown;
  isLoading: boolean;
};

export const CreateEditRolePageView: FC<CreateEditRolePageViewProps> = ({
  onSubmit,
  error,
  isLoading,
}) => {
  const navigate = useNavigate();
  const form = useFormik<Role>({
    initialValues: {
      name: "",
      display_name: "",
      site_permissions: [],
      organization_permissions: [],
      user_permissions: [],
    },
    validationSchema,
    onSubmit,
  });
  const getFieldHelpers = getFormHelpers<Role>(form, error);
  const onCancel = () => navigate(-1);

  return (
    <>
      <PageHeader css={{ paddingTop: 8 }}>
        <PageHeaderTitle>Create custom role</PageHeaderTitle>
      </PageHeader>
      <HorizontalForm onSubmit={form.handleSubmit}>
        <FormSection
          title="Role settings"
          description="Set a name for this role."
        >
          <FormFields>
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
            <ActionCheckboxes permissions={[]}></ActionCheckboxes>
          </FormFields>
        </FormSection>
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </HorizontalForm>
    </>
  );
};

interface ActionCheckboxesProps {
  permissions: Permissions[];
}

const ActionCheckboxes: FC<ActionCheckboxesProps> = ({ permissions }) => {
  return (
    <TableContainer>
      <Table>
        <TableBody>
          {Object.entries(RBACResourceActions).map(([key, value]) => {
            return (
              <TableRow key={key}>
                <TableCell>
                  <li key={key} css={styles.checkBoxes}>
                    <input type="checkbox" /> {key}
                    <ul css={styles.checkBoxes}>
                      {Object.entries(value).map(([key, value]) => {
                        return (
                          <li key={key}>
                            <span css={styles.actionText}>
                              <input type="checkbox" /> {key}
                            </span>{" "}
                            -{" "}
                            <span css={styles.actionDescription}>{value}</span>
                          </li>
                        );
                      })}
                    </ul>
                  </li>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

const styles = {
  rolesDropdown: {
    marginBottom: 20,
  },
  checkBoxes: {
    margin: 0,
    listStyleType: "none",
  },
  actionText: (theme) => ({
    color: theme.palette.text.primary,
  }),
  actionDescription: (theme) => ({
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export default CreateEditRolePageView;
