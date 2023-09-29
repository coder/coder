import { useMachine } from "@xstate/react";
import { User } from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { nonInitialPage } from "components/PaginationWidget/utils";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { FC, ReactNode, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useSearchParams, useNavigate } from "react-router-dom";
import { usersMachine } from "xServices/users/usersXService";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { ResetPasswordDialog } from "./ResetPasswordDialog";
import { pageTitle } from "utils/page";
import { UsersPageView } from "./UsersPageView";
import { useStatusFilterMenu } from "./UsersFilter";
import { useFilter } from "components/Filter/filter";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getAuthMethods } from "api/api";
import { roles } from "api/queries/roles";
import { deploymentConfig } from "api/queries/deployment";
import { prepareQuery } from "utils/filters";
import { usePagination } from "hooks";
import {
  users,
  suspendUser,
  activateUser,
  deleteUser,
  updatePassword,
} from "api/queries/users";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { getErrorMessage } from "api/errors";
import { generateRandomString } from "utils/random";

export const Language = {
  suspendDialogTitle: "Suspend user",
  suspendDialogAction: "Suspend",
  suspendDialogMessagePrefix: "Do you want to suspend the user",
  activateDialogTitle: "Activate user",
  activateDialogAction: "Activate",
  activateDialogMessagePrefix: "Do you want to activate the user",
};

export const UsersPage: FC<{ children?: ReactNode }> = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const searchParamsResult = useSearchParams();
  const { entitlements } = useDashboard();
  const [searchParams] = searchParamsResult;
  const filter = searchParams.get("filter") ?? "";
  const [usersState, usersSend] = useMachine(usersMachine);
  const pagination = usePagination({
    searchParamsResult,
  });
  const usersQuery = useQuery(
    users({
      q: prepareQuery(filter),
      limit: pagination.limit,
      offset: pagination.offset,
    }),
  );
  const { updateUsers: canEditUsers, viewDeploymentValues } = usePermissions();
  const rolesQuery = useQuery({ ...roles(), enabled: canEditUsers });
  const { data: deploymentValues } = useQuery({
    ...deploymentConfig(),
    enabled: viewDeploymentValues,
  });
  // Indicates if oidc roles are synced from the oidc idp.
  // Assign 'false' if unknown.
  const oidcRoleSyncEnabled =
    viewDeploymentValues &&
    deploymentValues?.config.oidc?.user_role_field !== "";
  const me = useMe();
  const useFilterResult = useFilter({
    searchParamsResult,
    onUpdate: () => {
      pagination.goToPage(1);
    },
  });
  const statusMenu = useStatusFilterMenu({
    value: useFilterResult.values.status,
    onChange: (option) =>
      useFilterResult.update({
        ...useFilterResult.values,
        status: option?.value,
      }),
  });
  const authMethods = useQuery({
    queryKey: ["authMethods"],
    queryFn: () => {
      return getAuthMethods();
    },
  });
  const isLoading =
    usersQuery.isLoading || rolesQuery.isLoading || authMethods.isLoading;
  // Suspend
  const [confirmSuspendUser, setConfirmSuspendUser] = useState<User>();
  const suspendUserMutation = useMutation(suspendUser(queryClient));
  // Activate
  const [confirmActivateUser, setConfirmActivateUser] = useState<User>();
  const activateUserMutation = useMutation(activateUser(queryClient));
  // Delete
  const [confirmDeleteUser, setConfirmDeleteUser] = useState<User>();
  const deleteUserMutation = useMutation(deleteUser(queryClient));
  // Reset password
  const [confirmResetPassword, setConfirmResetPassword] = useState<{
    user: User;
    newPassword: string;
  }>();
  const updatePasswordMutation = useMutation(updatePassword());

  return (
    <>
      <Helmet>
        <title>{pageTitle("Users")}</title>
      </Helmet>
      <UsersPageView
        oidcRoleSyncEnabled={oidcRoleSyncEnabled}
        roles={rolesQuery.data}
        users={usersQuery.data?.users}
        authMethods={authMethods.data}
        onListWorkspaces={(user) => {
          navigate(
            "/workspaces?filter=" +
              encodeURIComponent(`owner:${user.username}`),
          );
        }}
        onViewActivity={(user) => {
          navigate(
            "/audit?filter=" + encodeURIComponent(`username:${user.username}`),
          );
        }}
        onDeleteUser={setConfirmDeleteUser}
        onSuspendUser={setConfirmSuspendUser}
        onActivateUser={setConfirmActivateUser}
        onResetUserPassword={(user) => {
          setConfirmResetPassword({
            user,
            newPassword: generateRandomString(12),
          });
        }}
        onUpdateUserRoles={(user, roles) => {
          usersSend({
            type: "UPDATE_USER_ROLES",
            userId: user.id,
            roles,
          });
        }}
        isUpdatingUserRoles={usersState.matches("updatingUserRoles")}
        isLoading={isLoading}
        canEditUsers={canEditUsers}
        canViewActivity={entitlements.features.audit_log.enabled}
        isNonInitialPage={nonInitialPage(searchParams)}
        actorID={me.id}
        filterProps={{
          filter: useFilterResult,
          error: usersQuery.error,
          menus: {
            status: statusMenu,
          },
        }}
        count={usersQuery.data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
      />

      <DeleteDialog
        key={confirmDeleteUser?.username}
        isOpen={confirmDeleteUser !== undefined}
        confirmLoading={deleteUserMutation.isLoading}
        name={confirmDeleteUser?.username ?? ""}
        entity="user"
        onConfirm={async () => {
          try {
            await deleteUserMutation.mutateAsync(confirmDeleteUser!.id);
            setConfirmDeleteUser(undefined);
            displaySuccess("User deleted");
          } catch (e) {
            displayError(getErrorMessage(e, "Error deleting user"));
          }
        }}
        onCancel={() => {
          setConfirmDeleteUser(undefined);
        }}
      />

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={confirmSuspendUser !== undefined}
        confirmLoading={suspendUserMutation.isLoading}
        title="Suspend user"
        confirmText="Suspend"
        onConfirm={async () => {
          try {
            await suspendUserMutation.mutateAsync(confirmSuspendUser!.id);
            setConfirmSuspendUser(undefined);
            displaySuccess("User suspended");
          } catch (e) {
            displayError(getErrorMessage(e, "Error suspending user"));
          }
        }}
        onClose={() => {
          setConfirmSuspendUser(undefined);
        }}
        description={
          <>
            Do you want to suspend the user{" "}
            <strong>{confirmSuspendUser?.username ?? ""}</strong>?
          </>
        }
      />

      <ConfirmDialog
        type="success"
        hideCancel={false}
        open={confirmActivateUser !== undefined}
        confirmLoading={activateUserMutation.isLoading}
        title={Language.activateDialogTitle}
        confirmText={Language.activateDialogAction}
        onConfirm={async () => {
          try {
            await activateUserMutation.mutateAsync(confirmActivateUser!.id);
            setConfirmActivateUser(undefined);
            displaySuccess("User activated");
          } catch (e) {
            displayError(getErrorMessage(e, "Error activating user"));
          }
        }}
        onClose={() => {
          setConfirmActivateUser(undefined);
        }}
        description={
          <>
            {Language.activateDialogMessagePrefix}{" "}
            <strong>{confirmActivateUser?.username ?? ""}</strong>?
          </>
        }
      />

      <ResetPasswordDialog
        key={confirmResetPassword?.user.username}
        open={confirmResetPassword !== undefined}
        loading={updatePasswordMutation.isLoading}
        user={confirmResetPassword?.user}
        newPassword={confirmResetPassword?.newPassword}
        onClose={() => {
          setConfirmResetPassword(undefined);
        }}
        onConfirm={async () => {
          try {
            await updatePasswordMutation.mutateAsync({
              userId: confirmResetPassword!.user.id,
              password: confirmResetPassword!.newPassword,
              old_password: "",
            });
            setConfirmResetPassword(undefined);
            displaySuccess("Password reset");
          } catch (e) {
            displayError(getErrorMessage(e, "Error resetting password"));
          }
        }}
      />
    </>
  );
};

export default UsersPage;
