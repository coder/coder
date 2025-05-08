import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import type { AssignableRoles, Role } from "api/typesGenerated";
import { Button as ShadcnButton } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { AddIcon, AddOutlined } from "lucide-react";
import { EllipsisVertical } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { docs } from "utils/docs";
import { PermissionPillsList } from "./PermissionPillsList";

interface CustomRolesPageViewProps {
	builtInRoles: AssignableRoles[] | undefined;
	customRoles: AssignableRoles[] | undefined;
	onDeleteRole: (role: Role) => void;
	canCreateOrgRole: boolean;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	isCustomRolesEnabled: boolean;
}

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
	builtInRoles,
	customRoles,
	onDeleteRole,
	canCreateOrgRole,
	canUpdateOrgRole,
	canDeleteOrgRole,
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
				{canCreateOrgRole && isCustomRolesEnabled && (
					<Button component={RouterLink} startIcon={<AddIcon />} to="create">
						Create custom role
					</Button>
				)}
			</Stack>
			<RoleTable
				roles={customRoles}
				isCustomRolesEnabled={isCustomRolesEnabled}
				canCreateOrgRole={canCreateOrgRole}
				canUpdateOrgRole={canUpdateOrgRole}
				canDeleteOrgRole={canDeleteOrgRole}
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
				canCreateOrgRole={canCreateOrgRole}
				canUpdateOrgRole={canUpdateOrgRole}
				canDeleteOrgRole={canDeleteOrgRole}
				onDeleteRole={onDeleteRole}
			/>
		</Stack>
	);
};

interface RoleTableProps {
	roles: AssignableRoles[] | undefined;
	isCustomRolesEnabled: boolean;
	canCreateOrgRole: boolean;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	onDeleteRole: (role: Role) => void;
}

const RoleTable: FC<RoleTableProps> = ({
	roles,
	isCustomRolesEnabled,
	canCreateOrgRole,
	canUpdateOrgRole,
	canDeleteOrgRole,
	onDeleteRole,
}) => {
	const isLoading = roles === undefined;
	const isEmpty = Boolean(roles && roles.length === 0);
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">Name</TableHead>
					<TableHead className="w-3/5">Permissions</TableHead>
					<TableHead className="w-auto" />
				</TableRow>
			</TableHeader>
			<TableBody>
				<ChooseOne>
					<Cond condition={isLoading}>
						<TableLoader />
					</Cond>

					<Cond condition={isEmpty}>
						<TableRow className="h-14">
							<TableCell colSpan={999}>
								<EmptyState
									message="No custom roles yet"
									description={
										canCreateOrgRole && isCustomRolesEnabled
											? "Create your first custom role"
											: !isCustomRolesEnabled
												? "Upgrade to a premium license to create a custom role"
												: "You don't have permission to create a custom role"
									}
									cta={
										canCreateOrgRole &&
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
						{[...(roles ?? [])]
							.sort((a, b) => a.name.localeCompare(b.name))
							.map((role) => (
								<RoleRow
									key={role.name}
									role={role}
									canUpdateOrgRole={canUpdateOrgRole}
									canDeleteOrgRole={canDeleteOrgRole}
									onDelete={() => onDeleteRole(role)}
								/>
							))}
					</Cond>
				</ChooseOne>
			</TableBody>
		</Table>
	);
};

interface RoleRowProps {
	role: AssignableRoles;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	onDelete: () => void;
}

const RoleRow: FC<RoleRowProps> = ({
	role,
	onDelete,
	canUpdateOrgRole,
	canDeleteOrgRole,
}) => {
	const navigate = useNavigate();

	return (
		<TableRow data-testid={`role-${role.name}`} className="h-14">
			<TableCell>{role.display_name || role.name}</TableCell>

			<TableCell>
				<PermissionPillsList permissions={role.organization_permissions} />
			</TableCell>

			<TableCell>
				{!role.built_in && (canUpdateOrgRole || canDeleteOrgRole) && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<ShadcnButton
								size="icon-lg"
								variant="subtle"
								aria-label="Open menu"
							>
								<EllipsisVertical aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</ShadcnButton>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							{canUpdateOrgRole && (
								<DropdownMenuItem onClick={() => navigate(role.name)}>
									Edit
								</DropdownMenuItem>
							)}
							{canDeleteOrgRole && (
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={onDelete}
								>
									Delete&hellip;
								</DropdownMenuItem>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
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
