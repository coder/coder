import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import PersonAdd from "@mui/icons-material/PersonAddOutlined";
import Button from "@mui/material/Button";
import { type FC, Suspense } from "react";
import {
  Link as RouterLink,
  Outlet,
  useNavigate,
  useLocation,
} from "react-router-dom";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { TAB_PADDING_Y, TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { linkToUsers } from "modules/navigation";

export const UsersLayout: FC = () => {
  const { permissions } = useAuthenticated();
  const { experiments } = useDashboard();
  const navigate = useNavigate();
  const feats = useFeatureVisibility();
  const location = useLocation();
  const activeTab = location.pathname.endsWith("groups") ? "groups" : "users";

  const canViewOrganizations =
    feats.multiple_organizations && experiments.includes("multi-organization");

  return (
    <>
      <Margins>
        <PageHeader
          actions={
            <>
              {permissions.createUser && (
                <Button
                  onClick={() => {
                    navigate("/users/create");
                  }}
                  startIcon={<PersonAdd />}
                >
                  Create user
                </Button>
              )}
              {permissions.createGroup && feats.template_rbac && (
                <Button
                  component={RouterLink}
                  startIcon={<GroupAdd />}
                  to="/groups/create"
                >
                  Create group
                </Button>
              )}
            </>
          }
        >
          <PageHeaderTitle>Users</PageHeaderTitle>
        </PageHeader>
      </Margins>

      {!canViewOrganizations && (
        <Tabs
          css={{ marginBottom: 40, marginTop: -TAB_PADDING_Y }}
          active={activeTab}
        >
          <Margins>
            <TabsList>
              <TabLink to={linkToUsers} value="users">
                Users
              </TabLink>
              <TabLink to="/groups" value="groups">
                Groups
              </TabLink>
            </TabsList>
          </Margins>
        </Tabs>
      )}

      <Margins>
        <Suspense fallback={<Loader />}>
          <Outlet />
        </Suspense>
      </Margins>
    </>
  );
};
