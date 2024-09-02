import type { Interpolation, Theme } from "@emotion/react";
import { useTheme } from "@emotion/react";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { OIDCConfig } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { docs } from "utils/docs";

export type IdpSyncPageViewProps = {
	oidcConfig: OIDCConfig | undefined;
};

type CircleProps = {
	color: string;
	variant?: "solid" | "outlined";
};

const Circle: FC<CircleProps> = ({ color, variant = "solid" }) => {
	return (
		<div
			aria-hidden
			css={{
				width: 8,
				height: 8,
				backgroundColor: variant === "solid" ? color : undefined,
				border: variant === "outlined" ? `1px solid ${color}` : undefined,
				borderRadius: 9999,
			}}
		/>
	);
};

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({ oidcConfig }) => {
	const theme = useTheme();
	const {
		groups_field,
		user_role_field,
		group_regex_filter,
		group_auto_create,
	} = oidcConfig || {};
	return (
		<>
			<ChooseOne>
				<Cond condition={false}>
					<Paywall
						message="IdP Sync"
						description="Configure group and role mappings to manage permissions outside of Coder."
						documentationLink={docs("/admin/groups")}
					/>
				</Cond>
				<Cond>
					<Stack spacing={2} css={styles.fields}>
						{/* Semantically fieldset is used for forms. In the future this screen will allow
						 updates to these fields in a form */}
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Groups</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={3}>
								<h4>Sync Field</h4>
								<p css={styles.secondary}>
									{groups_field || (
										<Stack
											style={{ color: theme.palette.text.secondary }}
											direction="row"
											spacing={1}
											alignItems="center"
										>
											<Circle color={theme.roles.error.fill.solid} />
											<p>disabled</p>
										</Stack>
									)}
								</p>
								<h4>Regex Filter</h4>
								<p css={styles.secondary}>{group_regex_filter || "none"}</p>
								<h4>Auto Create</h4>
								<p css={styles.secondary}>
									{oidcConfig?.group_auto_create.toString()}
								</p>
							</Stack>
						</fieldset>
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Roles</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={3}>
								<h4>Sync Field</h4>
								<p css={styles.secondary}>
									{user_role_field || (
										<Stack
											style={{ color: theme.palette.text.secondary }}
											direction="row"
											spacing={1}
											alignItems="center"
										>
											<Circle color={theme.roles.error.fill.solid} />
											<p>disabled</p>
										</Stack>
									)}
								</p>
							</Stack>
						</fieldset>
					</Stack>
					<Stack spacing={6}>
						<IdpMappingTable
							type="Role"
							isEmpty={Boolean(
								!oidcConfig?.user_role_mapping ||
									(oidcConfig?.user_role_mapping &&
										Object.entries(oidcConfig?.user_role_mapping).length ===
											0) ||
									false,
							)}
						>
							<>
								{oidcConfig?.user_role_mapping &&
									Object.entries(oidcConfig.user_role_mapping).map(
										([idpRole, roles]) => (
											<RoleRow
												key={idpRole}
												idpRole={idpRole}
												coderRoles={roles}
											/>
										),
									)}
							</>
						</IdpMappingTable>
						<IdpMappingTable
							type="Group"
							isEmpty={Boolean(
								!oidcConfig?.group_mapping ||
									(oidcConfig?.group_mapping &&
										Object.entries(oidcConfig?.group_mapping).length === 0) ||
									false,
							)}
						>
							<>
								{oidcConfig?.user_role_mapping &&
									Object.entries(oidcConfig.group_mapping).map(
										([idpGroup, group]) => (
											<GroupRow
												key={idpGroup}
												idpGroup={idpGroup}
												coderGroup={group}
											/>
										),
									)}
							</>
						</IdpMappingTable>
					</Stack>
				</Cond>
			</ChooseOne>
		</>
	);
};

interface IdpMappingTableProps {
	type: "Role" | "Group";
	isEmpty: boolean;
	children: React.ReactNode;
}

const IdpMappingTable: FC<IdpMappingTableProps> = ({
	type,
	isEmpty,
	children,
}) => {
	const isLoading = false;

	return (
		<TableContainer>
			<Table>
				<TableHead>
					<TableRow>
						<TableCell width="45%">Idp {type}</TableCell>
						<TableCell width="55%">Coder {type}</TableCell>
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
										message={`No ${type} Mappings`}
										isCompact
										cta={
											<Button
												startIcon={<LaunchOutlined />}
												component="a"
												href={docs("/admin/auth#group-sync-enterprise")}
												target="_blank"
											>
												How to setup IdP {type} sync
											</Button>
										}
									/>
								</TableCell>
							</TableRow>
						</Cond>

						<Cond>{children}</Cond>
					</ChooseOne>
				</TableBody>
			</Table>
		</TableContainer>
	);
};

interface GroupRowProps {
	idpGroup: string;
	coderGroup: string;
}

const GroupRow: FC<GroupRowProps> = ({ idpGroup, coderGroup }) => {
	return (
		<TableRow data-testid={`group-${idpGroup}`}>
			<TableCell>{idpGroup}</TableCell>
			<TableCell css={styles.secondary}>{coderGroup}</TableCell>
		</TableRow>
	);
};

interface RoleRowProps {
	idpRole: string;
	coderRoles: ReadonlyArray<string>;
}

const RoleRow: FC<RoleRowProps> = ({ idpRole, coderRoles }) => {
	return (
		<TableRow data-testid={`role-${idpRole}`}>
			<TableCell>{idpRole}</TableCell>
			<TableCell css={styles.secondary}>coderRoles Placeholder</TableCell>
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
	fields: () => ({
		marginBottom: "60px",
	}),
	legend: () => ({
		padding: "0px 6px",
		fontWeight: 600,
	}),
	box: (theme) => ({
		border: "1px solid",
		borderColor: theme.palette.divider,
		padding: "0px 20px",
		borderRadius: 8,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default IdpSyncPageView;
