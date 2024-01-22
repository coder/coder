import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import PersonAdd from "@mui/icons-material/PersonAddOutlined";
import { type FC, Suspense } from "react";
import { Link as RouterLink, Outlet, useNavigate } from "react-router-dom";
import { usePermissions } from "contexts/auth/usePermissions";
import { USERS_LINK } from "modules/dashboard/Navbar/NavbarView";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Margins } from "components/Margins/Margins";
import { TabLink, Tabs } from "components/Tabs/Tabs";
import { Loader } from "components/Loader/Loader";

export const UsersLayout: FC = () => {
  const { createUser: canCreateUser, createGroup: canCreateGroup } =
    usePermissions();
  const navigate = useNavigate();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();

  return (
    <>
      <Margins>
        <PageHeader
          actions={
            <>
              {canCreateUser && (
                <Button
                  onClick={() => {
                    navigate("/users/create");
                  }}
                  startIcon={<PersonAdd />}
                >
                  Create user
                </Button>
              )}
              {canCreateGroup && isTemplateRBACEnabled && (
                <Link component={RouterLink} to="/groups/create">
                  <Button startIcon={<GroupAdd />}>Create group</Button>
                </Link>
              )}
            </>
          }
        >
          <PageHeaderTitle>Users</PageHeaderTitle>
        </PageHeader>
      </Margins>

      <Tabs>
        <TabLink to={USERS_LINK}>Users</TabLink>
        <TabLink to="/groups">Groups</TabLink>
      </Tabs>

      <Margins>
        <Suspense fallback={<Loader />}>
          <Outlet />
        </Suspense>
      </Margins>
    </>
  );
};
