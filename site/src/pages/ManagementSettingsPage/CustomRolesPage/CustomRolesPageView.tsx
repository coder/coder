import type { Interpolation, Theme } from "@emotion/react";
import AddOutlined from "@mui/icons-material/AddOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { Role } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	MoreMenu,
	MoreMenuContent,
	MoreMenuItem,
	MoreMenuTrigger,
	ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { docs } from "utils/docs";
import { PermissionPillsList } from "./PermissionPillsList";

interface CustomRolesPageViewProps {
	roles: Role[] | undefined;
	onDeleteRole: (role: Role) => void;
	canAssignOrgRole: boolean;
	isCustomRolesEnabled: boolean;
}

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
	roles,
	onDeleteRole,
	canAssignOrgRole,
	isCustomRolesEnabled,
}) => {
	const isLoading = roles === undefined;
	const isEmpty = Boolean(roles && roles.length === 0);
	return (
		<Stack spacing={4}>
			{!isCustomRolesEnabled && (
				<Paywall
					message="Custom Roles"
					description="Create custom roles to grant users a tailored set of granular permissions."
					documentationLink={docs("/admin/groups")}
				/>
			)}
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="40%">Name</TableCell>
							<TableCell width="59%">Permissions</TableCell>
							<TableCell width="1%" />
						</TableRow>
					</TableHead>
					<TableBody>
						<ChooseOne>
							<Cond condition={isLoading}>
								<TableLoader />
							</Cond>

							<Cond condition={isEmpty}>
								<TableRow>
									<TableCell colSpan={999}>
										<EmptyState
											message="No custom roles yet"
											description={
												canAssignOrgRole && isCustomRolesEnabled
													? "Create your first custom role"
													: !isCustomRolesEnabled
														? "Upgrade to a premium license to create a custom role"
														: "You don't have permission to create a custom role"
											}
											cta={
												canAssignOrgRole &&
												isCustomRolesEnabled && (
													<Button
														component={RouterLink}
														to="create"
														startIcon={<AddOutlined />}
														variant="contained"
													>
														Create custom role
													</Button>
												)
											}
										/>
									</TableCell>
								</TableRow>
							</Cond>

							<Cond>
								{roles?.map((role) => (
									<RoleRow
										key={role.name}
										role={role}
										canAssignOrgRole={canAssignOrgRole}
										onDelete={() => onDeleteRole(role)}
									/>
								))}
							</Cond>
						</ChooseOne>
					</TableBody>
				</Table>
			</TableContainer>
		</Stack>
	);
};

interface RoleRowProps {
	role: Role;
	onDelete: () => void;
	canAssignOrgRole: boolean;
}

const RoleRow: FC<RoleRowProps> = ({ role, onDelete, canAssignOrgRole }) => {
	const navigate = useNavigate();

	return (
		<TableRow data-testid={`role-${role.name}`}>
			<TableCell>{role.display_name || role.name}</TableCell>

			<TableCell>
				<PermissionPillsList permissions={role.organization_permissions} />
			</TableCell>

			<TableCell>
				<MoreMenu>
					<MoreMenuTrigger>
						<ThreeDotsButton />
					</MoreMenuTrigger>
					<MoreMenuContent>
						<MoreMenuItem
							onClick={() => {
								navigate(role.name);
							}}
						>
							Edit
						</MoreMenuItem>
						{canAssignOrgRole && (
							<MoreMenuItem danger onClick={onDelete}>
								Delete&hellip;
							</MoreMenuItem>
						)}
					</MoreMenuContent>
				</MoreMenu>
			</TableCell>
		</TableRow>
	);
};

const TableLoader = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

const styles = {
	secondary: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default CustomRolesPageView;
