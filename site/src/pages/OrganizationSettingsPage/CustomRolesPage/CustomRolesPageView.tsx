import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import AddIcon from "@mui/icons-material/AddOutlined";
import AddOutlined from "@mui/icons-material/AddOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { AssignableRoles, Role } from "api/typesGenerated";
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
	builtInRoles: AssignableRoles[] | undefined;
	customRoles: AssignableRoles[] | undefined;
	onDeleteRole: (role: Role) => void;
	canAssignOrgRole: boolean;
	isCustomRolesEnabled: boolean;
}

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
	builtInRoles,
	customRoles,
	onDeleteRole,
	canAssignOrgRole,
	isCustomRolesEnabled,
}) => {
	return (
		<Stack spacing={4}>
			{!isCustomRolesEnabled && (
				<Paywall
					message="Custom Roles"
					description="Create custom roles to grant users a tailored set of granular permissions."
					documentationLink={docs("/admin/users/groups-roles")}
				/>
			)}
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<span>
					<h2 css={styles.tableHeader}>Custom Roles</h2>
					<span css={styles.tableDescription}>
						Create custom roles to grant users a tailored set of granular
						permissions.
					</span>
				</span>
				{canAssignOrgRole && isCustomRolesEnabled && (
					<Button component={RouterLink} startIcon={<AddIcon />} to="create">
						Create custom role
					</Button>
				)}
			</Stack>
			<RoleTable
				roles={customRoles}
				isCustomRolesEnabled={isCustomRolesEnabled}
				canAssignOrgRole={canAssignOrgRole}
				onDeleteRole={onDeleteRole}
			/>
			<span>
				<h2 css={styles.tableHeader}>Built-In Roles</h2>
				<span css={styles.tableDescription}>
					Built-in roles have predefined permissions. You cannot edit or delete
					built-in roles.
				</span>
			</span>
			<RoleTable
				roles={builtInRoles}
				isCustomRolesEnabled={isCustomRolesEnabled}
				canAssignOrgRole={canAssignOrgRole}
				onDeleteRole={onDeleteRole}
			/>
		</Stack>
	);
};

interface RoleTableProps {
	roles: AssignableRoles[] | undefined;
	isCustomRolesEnabled: boolean;
	canAssignOrgRole: boolean;
	onDeleteRole: (role: Role) => void;
}

const RoleTable: FC<RoleTableProps> = ({
	roles,
	isCustomRolesEnabled,
	onDeleteRole,
	canAssignOrgRole,
}) => {
	const isLoading = roles === undefined;
	const isEmpty = Boolean(roles && roles.length === 0);
	return (
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
							{roles
								?.sort((a, b) => a.name.localeCompare(b.name))
								.map((role) => (
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
	);
};

interface RoleRowProps {
	role: AssignableRoles;
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
				{!role.built_in && (
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
				)}
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
	tableHeader: () => ({
		marginBottom: 0,
		fontSize: 18,
	}),
	tableDescription: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
		lineHeight: "160%",
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default CustomRolesPageView;
