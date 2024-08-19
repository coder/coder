import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import AddOutlined from "@mui/icons-material/AddOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Stack from "@mui/material/Stack";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { Permission, Role } from "api/typesGenerated";
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
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { docs } from "utils/docs";

export type CustomRolesPageViewProps = {
	roles: Role[] | undefined;
	onDeleteRole: (role: Role) => void;
	canAssignOrgRole: boolean;
	isCustomRolesEnabled: boolean;
};

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
	roles,
	onDeleteRole,
	canAssignOrgRole,
	isCustomRolesEnabled,
}) => {
	const isLoading = roles === undefined;
	const isEmpty = Boolean(roles && roles.length === 0);
	return (
		<>
			<ChooseOne>
				<Cond condition={!isCustomRolesEnabled}>
					<Paywall
						message="Custom Roles"
						description="Create custom roles to assign a specific set of permissions to a user. You need an Enterprise license to use this feature."
						documentationLink={docs("/admin/groups")}
					/>
				</Cond>
				<Cond>
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
														canAssignOrgRole
															? "Create your first custom role"
															: "You don't have permission to create a custom role"
													}
													cta={
														canAssignOrgRole && (
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
				</Cond>
			</ChooseOne>
		</>
	);
};

function getUniqueResourceTypes(jsonObject: readonly Permission[]) {
	const resourceTypes = jsonObject.map((item) => item.resource_type);
	return [...new Set(resourceTypes)];
}
interface RoleRowProps {
	role: Role;
	onDelete: () => void;
	canAssignOrgRole: boolean;
}

const RoleRow: FC<RoleRowProps> = ({ role, onDelete, canAssignOrgRole }) => {
	const navigate = useNavigate();

	const resourceTypes: string[] = getUniqueResourceTypes(
		role.organization_permissions,
	);

	return (
		<TableRow data-testid={`role-${role.name}`}>
			<TableCell>{role.display_name || role.name}</TableCell>

			<TableCell>
				<Stack direction="row" spacing={1}>
					<PermissionsPill
						resource={resourceTypes[0]}
						permissions={role.organization_permissions}
					/>

					{resourceTypes.length > 1 && (
						<OverflowPermissionPill
							resources={resourceTypes.slice(1)}
							permissions={role.organization_permissions.slice(1)}
						/>
					)}
				</Stack>
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

interface PermissionPillProps {
	resource: string;
	permissions: readonly Permission[];
}

const PermissionsPill: FC<PermissionPillProps> = ({
	resource,
	permissions,
}) => {
	const actions = permissions.filter((p) => {
		if (resource === p.resource_type) {
			return p.action;
		}
	});

	return (
		<Pill css={styles.permissionPill}>
			<b>{resource}</b>: {actions.map((p) => p.action).join(", ")}
		</Pill>
	);
};

type OverflowPermissionPillProps = {
	resources: string[];
	permissions: readonly Permission[];
};

const OverflowPermissionPill: FC<OverflowPermissionPillProps> = ({
	resources,
	permissions,
}) => {
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					css={{
						backgroundColor: theme.palette.background.paper,
						borderColor: theme.palette.divider,
					}}
				>
					+{resources.length} more
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				disableRestoreFocus
				disableScrollLock
				css={{
					".MuiPaper-root": {
						display: "flex",
						flexFlow: "column wrap",
						columnGap: 8,
						rowGap: 12,
						padding: "12px 16px",
						alignContent: "space-around",
						minWidth: "auto",
						backgroundColor: theme.palette.background.default,
					},
				}}
				anchorOrigin={{
					vertical: -4,
					horizontal: "center",
				}}
				transformOrigin={{
					vertical: "bottom",
					horizontal: "center",
				}}
			>
				{resources.map((resource) => (
					<PermissionsPill
						key={resource}
						resource={resource}
						permissions={permissions}
					/>
				))}
			</PopoverContent>
		</Popover>
	);
};

const styles = {
	secondary: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	permissionPill: (theme) => ({
		backgroundColor: theme.permission.background,
		borderColor: theme.permission.outline,
		color: theme.permission.text,
		width: "fit-content",
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default CustomRolesPageView;
