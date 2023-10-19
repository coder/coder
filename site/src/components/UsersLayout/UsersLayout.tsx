import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import PersonAdd from "@mui/icons-material/PersonAddOutlined";
import { USERS_LINK } from "components/Dashboard/Navbar/NavbarView";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { usePermissions } from "hooks/usePermissions";
import { FC } from "react";
import { Link as RouterLink, Outlet, useNavigate } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { TabLink, Tabs } from "components/Tabs/Tabs";

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
        <Outlet />
      </Margins>
    </>
  );
};
