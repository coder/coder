import type { Interpolation, Theme } from "@emotion/react";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import TextField from "@mui/material/TextField";
import type { useFormik } from "formik";
import { type ChangeEvent, useState, type FC } from "react";
import { useNavigate } from "react-router-dom";
import { isApiValidationError } from "api/errors";
import { RBACResourceActions } from "api/rbacresources_gen";
import type {
  Role,
  PatchRoleRequest,
  Permission,
  AssignableRoles,
  RBACResource,
  RBACAction,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { getFormHelpers } from "utils/formUtils";

export type CreateEditRolePageViewProps = {
  role: AssignableRoles | undefined;
  form: ReturnType<typeof useFormik<PatchRoleRequest>>;
  error?: unknown;
  isLoading: boolean;
};

export const CreateEditRolePageView: FC<CreateEditRolePageViewProps> = ({
  role,
  form,
  error,
  isLoading,
}) => {
  const navigate = useNavigate();
  const getFieldHelpers = getFormHelpers<Role>(form, error);
  const onCancel = () => navigate(-1);

  return (
    <>
      <HorizontalForm onSubmit={form.handleSubmit}>
        <FormSection
          title="Role settings"
          description="Set a name and permissions for this role."
        >
          <FormFields>
            {Boolean(error) && !isApiValidationError(error) && (
              <ErrorAlert error={error} />
            )}

            <TextField
              {...getFieldHelpers("name", {
                helperText:
                  "The role name cannot be modified after the role is created.",
              })}
              autoFocus
              fullWidth
              disabled={role !== undefined}
              label="Name"
            />
            <TextField
              {...getFieldHelpers("display_name", {
                helperText: "Optional: keep empty to default to the name.",
              })}
              fullWidth
              label="Display Name"
            />
            <ActionCheckboxes
              permissions={role?.organization_permissions || []}
              form={form}
            />
          </FormFields>
        </FormSection>
        <FormFooter
          onCancel={onCancel}
          isLoading={isLoading}
          submitLabel={role !== undefined ? "Save" : "Create Role"}
        />
      </HorizontalForm>
    </>
  );
};

interface ActionCheckboxesProps {
  permissions: readonly Permission[] | undefined;
  form: ReturnType<typeof useFormik<Role>> & { values: Role };
}

const ResourceActionComparator = (
  p: Permission,
  resource: string,
  action: string,
) =>
  p.resource_type === resource &&
  (p.action.toString() === "*" || p.action === action);

const DEFAULT_RESOURCES = [
  "audit_log",
  "group",
  "template",
  "organization_member",
  "provisioner_daemon",
  "workspace",
];

const resources = new Set(DEFAULT_RESOURCES);

const filteredRBACResourceActions = Object.fromEntries(
  Object.entries(RBACResourceActions).filter(([resource]) =>
    resources.has(resource),
  ),
);

const ActionCheckboxes: FC<ActionCheckboxesProps> = ({ permissions, form }) => {
  const [checkedActions, setCheckActions] = useState(permissions);
  const [showAllResources, setShowAllResources] = useState(false);

  const handleActionCheckChange = async (
    e: ChangeEvent<HTMLInputElement>,
    form: ReturnType<typeof useFormik<Role>> & { values: Role },
  ) => {
    const { name, checked } = e.currentTarget;
    const [resource_type, action] = name.split(":");

    const newPermissions = checked
      ? [
          ...(checkedActions ?? []),
          {
            negate: false,
            resource_type: resource_type as RBACResource,
            action: action as RBACAction,
          },
        ]
      : checkedActions?.filter(
          (p) => p.resource_type !== resource_type || p.action !== action,
        );

    setCheckActions(newPermissions);
    await form.setFieldValue("organization_permissions", newPermissions);
  };

  const resourceActions = showAllResources
    ? RBACResourceActions
    : filteredRBACResourceActions;

  return (
    <>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell
                align="right"
                sx={{ paddingTop: 0.4, paddingBottom: 0.4 }}
              >
                <FormControlLabel
                  sx={{ marginRight: 1 }}
                  control={
                    <Checkbox
                      size="small"
                      id="show_all_permissions"
                      name="show_all_permissions"
                      checked={showAllResources}
                      onChange={(e) =>
                        setShowAllResources(e.currentTarget.checked)
                      }
                    />
                  }
                  label={
                    <span style={{ fontSize: 12 }}>Show all permissions</span>
                  }
                />
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.entries(resourceActions).map(([resourceKey, value]) => {
              return (
                <TableRow key={resourceKey}>
                  <TableCell sx={{ paddingLeft: 2 }}>
                    <li key={resourceKey} css={styles.checkBoxes}>
                      {resourceKey}
                      <ul css={styles.checkBoxes}>
                        {Object.entries(value).map(([actionKey, value]) => {
                          return (
                            <li key={actionKey}>
                              <span css={styles.actionText}>
                                <Checkbox
                                  name={`${resourceKey}:${actionKey}`}
                                  checked={
                                    checkedActions?.some((p) =>
                                      ResourceActionComparator(
                                        p,
                                        resourceKey,
                                        actionKey,
                                      ),
                                    ) || false
                                  }
                                  onChange={(e) =>
                                    handleActionCheckChange(e, form)
                                  }
                                />
                                {actionKey}
                              </span>{" "}
                              &ndash;{" "}
                              <span css={styles.actionDescription}>
                                {value}
                              </span>
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
    </>
  );
};

const styles = {
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
