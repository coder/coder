import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import { useMachine } from "@xstate/react";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { templateACLMachine } from "xServices/template/templateACLXService";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";
import { docs } from "utils/docs";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { setUserRole, templateACL } from "api/queries/templates";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const TemplatePermissionsPage: FC<
  React.PropsWithChildren<unknown>
> = () => {
  const organizationId = useOrganizationId();
  const { template, permissions } = useTemplateSettings();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const [state, send] = useMachine(templateACLMachine, {
    context: { templateId: template.id },
  });
  const { groupToBeUpdated } = state.context;
  const templateACLQuery = useQuery({
    ...templateACL(template.id),
    onSuccess: (data) => {
      send({ type: "LOAD", data });
    },
  });
  const queryClient = useQueryClient();
  const addUserMutation = useMutation(setUserRole(queryClient));
  const updateUserMutation = useMutation(setUserRole(queryClient));
  const removeUserMutation = useMutation(setUserRole(queryClient));

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "Permissions"])}</title>
      </Helmet>
      {!isTemplateRBACEnabled ? (
        <Paywall
          message="Template permissions"
          description="Manage your template permissions to allow users or groups to view or admin the template. To use this feature, you have to upgrade your account."
          cta={
            <Stack direction="row" alignItems="center">
              <Link
                href={docs("/admin/upgrade")}
                target="_blank"
                rel="noreferrer"
              >
                <Button
                  startIcon={<ArrowRightAltOutlined />}
                  variant="contained"
                >
                  See how to upgrade
                </Button>
              </Link>
              <Link href={docs("/admin/rbac")} target="_blank" rel="noreferrer">
                Read the docs
              </Link>
            </Stack>
          }
        />
      ) : (
        <TemplatePermissionsPageView
          organizationId={organizationId}
          templateID={template.id}
          templateACL={templateACLQuery.data}
          canUpdatePermissions={Boolean(permissions?.canUpdateTemplate)}
          onAddUser={async (user, role, reset) => {
            await addUserMutation.mutateAsync({
              templateId: template.id,
              userId: user.id,
              role,
            });
            reset();
          }}
          isAddingUser={addUserMutation.isLoading}
          onUpdateUser={async (user, role) => {
            await updateUserMutation.mutateAsync({
              templateId: template.id,
              userId: user.id,
              role,
            });
            displaySuccess("User role updated successfully!");
          }}
          updatingUserId={
            updateUserMutation.isLoading
              ? updateUserMutation.variables?.userId
              : undefined
          }
          onRemoveUser={async (user) => {
            await removeUserMutation.mutateAsync({
              templateId: template.id,
              userId: user.id,
              role: "",
            });
            displaySuccess("User removed successfully!");
          }}
          onAddGroup={(group, role, reset) => {
            send("ADD_GROUP", { group, role, onDone: reset });
          }}
          isAddingGroup={state.matches("addingGroup")}
          onUpdateGroup={(group, role) => {
            send("UPDATE_GROUP_ROLE", { group, role });
          }}
          updatingGroup={groupToBeUpdated}
          onRemoveGroup={(group) => {
            send("REMOVE_GROUP", { group });
          }}
        />
      )}
    </>
  );
};

export default TemplatePermissionsPage;
