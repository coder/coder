import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { setGroupRole, setUserRole, templateACL } from "api/queries/templates";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { pageTitle } from "utils/page";
import { docs } from "utils/docs";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";

export const TemplatePermissionsPage: FC<
  React.PropsWithChildren<unknown>
> = () => {
  const organizationId = useOrganizationId();
  const { template, permissions } = useTemplateSettings();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const templateACLQuery = useQuery(templateACL(template.id));
  const queryClient = useQueryClient();

  const addUserMutation = useMutation(setUserRole(queryClient));
  const updateUserMutation = useMutation(setUserRole(queryClient));
  const removeUserMutation = useMutation(setUserRole(queryClient));

  const addGroupMutation = useMutation(setGroupRole(queryClient));
  const updateGroupMutation = useMutation(setGroupRole(queryClient));
  const removeGroupMutation = useMutation(setGroupRole(queryClient));

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
                Read the documentation
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
          onAddGroup={async (group, role, reset) => {
            await addGroupMutation.mutateAsync({
              templateId: template.id,
              groupId: group.id,
              role,
            });
            reset();
          }}
          isAddingGroup={addGroupMutation.isLoading}
          onUpdateGroup={async (group, role) => {
            await updateGroupMutation.mutateAsync({
              templateId: template.id,
              groupId: group.id,
              role,
            });
            displaySuccess("Group role updated successfully!");
          }}
          updatingGroupId={
            updateGroupMutation.isLoading
              ? updateGroupMutation.variables?.groupId
              : undefined
          }
          onRemoveGroup={async (group) => {
            await removeGroupMutation.mutateAsync({
              groupId: group.id,
              templateId: template.id,
              role: "",
            });
            displaySuccess("Group removed successfully!");
          }}
        />
      )}
    </>
  );
};

export default TemplatePermissionsPage;
