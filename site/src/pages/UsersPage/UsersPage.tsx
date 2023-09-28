import { useMachine } from "@xstate/react";
import { User } from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { nonInitialPage } from "components/PaginationWidget/utils";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { FC, ReactNode } from "react";
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
import { useQuery } from "@tanstack/react-query";
import { getAuthMethods } from "api/api";
import { roles } from "api/queries/roles";
import { deploymentConfig } from "api/queries/deployment";
import { prepareQuery } from "utils/filters";
import { usePagination } from "hooks";
import * as UsersQuery from "api/queries/users";

export const Language = {
  suspendDialogTitle: "Suspend user",
  suspendDialogAction: "Suspend",
  suspendDialogMessagePrefix: "Do you want to suspend the user",
  activateDialogTitle: "Activate user",
  activateDialogAction: "Activate",
  activateDialogMessagePrefix: "Do you want to activate the user",
};

const getSelectedUser = (id: string, users?: User[]) =>
  users?.find((u) => u.id === id);

export const UsersPage: FC<{ children?: ReactNode }> = () => {
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
    UsersQuery.users({
      q: prepareQuery(filter),
      limit: pagination.limit,
      offset: pagination.offset,
    }),
  );
  const users = usersQuery.data?.users;
  const count = usersQuery.data?.count;
  const {
    usernameToDelete,
    usernameToSuspend,
    usernameToActivate,
    userIdToResetPassword,
    newUserPassword,
  } = usersState.context;
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

  return (
    <>
      <Helmet>
        <title>{pageTitle("Users")}</title>
      </Helmet>
      <UsersPageView
        oidcRoleSyncEnabled={oidcRoleSyncEnabled}
        roles={rolesQuery.data}
        users={users}
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
        onDeleteUser={(user) => {
          usersSend({
            type: "DELETE_USER",
            userId: user.id,
            username: user.username,
          });
        }}
        onSuspendUser={(user) => {
          usersSend({
            type: "SUSPEND_USER",
            userId: user.id,
            username: user.username,
          });
        }}
        onActivateUser={(user) => {
          usersSend({
            type: "ACTIVATE_USER",
            userId: user.id,
            username: user.username,
          });
        }}
        onResetUserPassword={(user) => {
          usersSend({ type: "RESET_USER_PASSWORD", userId: user.id });
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
        count={count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
      />

      <DeleteDialog
        key={usernameToDelete}
        isOpen={
          usersState.matches("confirmUserDeletion") ||
          usersState.matches("deletingUser")
        }
        confirmLoading={usersState.matches("deletingUser")}
        name={usernameToDelete ?? ""}
        entity="user"
        onConfirm={() => {
          usersSend("CONFIRM_USER_DELETE");
        }}
        onCancel={() => {
          usersSend("CANCEL_USER_DELETE");
        }}
      />

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={
          usersState.matches("confirmUserSuspension") ||
          usersState.matches("suspendingUser")
        }
        confirmLoading={usersState.matches("suspendingUser")}
        title={Language.suspendDialogTitle}
        confirmText={Language.suspendDialogAction}
        onConfirm={() => {
          usersSend("CONFIRM_USER_SUSPENSION");
        }}
        onClose={() => {
          usersSend("CANCEL_USER_SUSPENSION");
        }}
        description={
          <>
            {Language.suspendDialogMessagePrefix}
            {usernameToSuspend && " "}
            <strong>{usernameToSuspend ?? ""}</strong>?
          </>
        }
      />

      <ConfirmDialog
        type="success"
        hideCancel={false}
        open={
          usersState.matches("confirmUserActivation") ||
          usersState.matches("activatingUser")
        }
        confirmLoading={usersState.matches("activatingUser")}
        title={Language.activateDialogTitle}
        confirmText={Language.activateDialogAction}
        onConfirm={() => {
          usersSend("CONFIRM_USER_ACTIVATION");
        }}
        onClose={() => {
          usersSend("CANCEL_USER_ACTIVATION");
        }}
        description={
          <>
            {Language.activateDialogMessagePrefix}
            {usernameToActivate && " "}
            <strong>{usernameToActivate ?? ""}</strong>?
          </>
        }
      />

      {userIdToResetPassword && (
        <ResetPasswordDialog
          open={
            usersState.matches("confirmUserPasswordReset") ||
            usersState.matches("resettingUserPassword")
          }
          loading={usersState.matches("resettingUserPassword")}
          user={getSelectedUser(userIdToResetPassword, users)}
          newPassword={newUserPassword}
          onClose={() => {
            usersSend("CANCEL_USER_PASSWORD_RESET");
          }}
          onConfirm={() => {
            usersSend("CONFIRM_USER_PASSWORD_RESET");
          }}
        />
      )}
    </>
  );
};

export default UsersPage;
