import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, Suspense } from "react";
import { Outlet, Link as RouterLink } from "react-router-dom";

export const UsersLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const feats = useFeatureVisibility();

	return (
		<>
			<Margins>
				<PageHeader
					actions={
						<div>
							{permissions.createGroup && feats.template_rbac && (
								<Button
									component={RouterLink}
									startIcon={<GroupAdd />}
									to="/deployment/groups/create"
								>
									Create group
								</Button>
							)}
						</div>
					}
				>
					<PageHeaderTitle>Groups</PageHeaderTitle>
				</PageHeader>
			</Margins>

			<Margins>
				<Suspense fallback={<Loader />}>
					<Outlet />
				</Suspense>
			</Margins>
		</>
	);
};
