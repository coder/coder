import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import PersonAdd from "@mui/icons-material/PersonAddOutlined";
import Button from "@mui/material/Button";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { TAB_PADDING_Y, TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { linkToUsers } from "modules/navigation";
import { type FC, Suspense } from "react";
import {
	Outlet,
	Link as RouterLink,
	useLocation,
	useNavigate,
} from "react-router-dom";

export const UsersLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const { showOrganizations } = useDashboard();
	const navigate = useNavigate();
	const feats = useFeatureVisibility();
	const location = useLocation();
	const activeTab = location.pathname.endsWith("groups") ? "groups" : "users";

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

			{!showOrganizations && (
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
