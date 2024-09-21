import type { Interpolation, Theme } from "@emotion/react";
import AddIcon from "@mui/icons-material/AddOutlined";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type {
	Group,
	GroupSyncSettings,
	RoleSyncSettings,
} from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import type { FC } from "react";
import { useSearchParams } from "react-router-dom";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { docs } from "utils/docs";
import { PillList } from "./PillList";

export type IdpSyncPageViewProps = {
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	groups: Group[] | undefined;
};

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	groupSyncSettings,
	roleSyncSettings,
	groups,
}) => {
	const [searchParams] = useSearchParams();
	const groupsMap = new Map<string, string>();
	if (groups) {
		for (const group of groups) {
			groupsMap.set(group.id, group.display_name || group.name);
		}
	}

	const getGroupNames = (groupIds: readonly string[]) => {
		return groupIds.map((groupId) => groupsMap.get(groupId) || groupId);
	};

	const tab = searchParams.get("tab") || "groups";

	const groupMappingCount = groupSyncSettings?.mapping
		? Object.entries(groupSyncSettings.mapping).length
		: 0;
	const roleMappingCount = roleSyncSettings?.mapping
		? Object.entries(roleSyncSettings.mapping).length
		: 0;

	return (
		<>
			<ChooseOne>
				<Cond condition={false}>
					<Paywall
						message="IdP Sync"
						description="Configure group and role mappings to manage permissions outside of Coder. You need an Premium license to use this feature."
						documentationLink={docs("/admin/groups")}
					/>
				</Cond>
				<Cond>
					<>
						<Tabs
							active={tab}
							css={{
								marginBottom: 24,
							}}
						>
							<TabsList>
								<TabLink to="?tab=groups" value="groups">
									Group Sync Settings
								</TabLink>
								<TabLink to="?tab=roles" value="roles">
									Role Sync Settings
								</TabLink>
							</TabsList>
						</Tabs>
						{tab === "groups" ? (
							<>
								<div css={styles.fields}>
									<Stack direction={"row"} alignItems={"center"} spacing={6}>
										<IdpField
											name={"Sync Field"}
											fieldText={groupSyncSettings?.field}
											showStatusIndicator
										/>
										<IdpField
											name={"Regex Filter"}
											fieldText={
												typeof groupSyncSettings?.regex_filter === "string"
													? groupSyncSettings?.regex_filter
													: "none"
											}
										/>
										<IdpField
											name={"Auto Create"}
											fieldText={String(
												groupSyncSettings?.auto_create_missing_groups || "n/a",
											)}
										/>
									</Stack>
								</div>
								<Stack
									direction="row"
									alignItems="baseline"
									justifyContent="space-between"
									css={styles.tableInfo}
								>
									<TableRowCount count={groupMappingCount} type="groups" />
									<Button
										component="a"
										startIcon={<AddIcon />}
										// to="export"
										href={docs("/admin/auth#group-sync-enterprise")}
									>
										Export Policy
									</Button>
								</Stack>
								<Stack spacing={6}>
									<IdpMappingTable
										type="Group"
										isEmpty={Boolean(groupMappingCount === 0)}
									>
										{groupSyncSettings?.mapping &&
											Object.entries(groupSyncSettings.mapping)
												.sort()
												.map(([idpGroup, groups]) => (
													<GroupRow
														key={idpGroup}
														idpGroup={idpGroup}
														coderGroup={getGroupNames(groups)}
													/>
												))}
									</IdpMappingTable>
								</Stack>
							</>
						) : (
							<>
								<div css={styles.fields}>
									<IdpField
										name={"Sync Field"}
										fieldText={roleSyncSettings?.field}
										showStatusIndicator
									/>
								</div>
								<Stack
									direction="row"
									alignItems="baseline"
									justifyContent="space-between"
									css={styles.tableInfo}
								>
									<TableRowCount count={roleMappingCount} type="roles" />
									<Button
										component="a"
										startIcon={<AddIcon />}
										// to="export"
										href={docs("/admin/auth#group-sync-enterprise")}
									>
										Export Policy
									</Button>
								</Stack>
								<IdpMappingTable
									type="Role"
									isEmpty={Boolean(roleMappingCount === 0)}
								>
									{roleSyncSettings?.mapping &&
										Object.entries(roleSyncSettings.mapping)
											.sort()
											.map(([idpRole, roles]) => (
												<RoleRow
													key={idpRole}
													idpRole={idpRole}
													coderRoles={roles}
												/>
											))}
								</IdpMappingTable>
							</>
						)}
					</>
				</Cond>
			</ChooseOne>
		</>
	);
};

interface IdpFieldProps {
	name: string;
	fieldText: string | undefined;
	showStatusIndicator?: boolean;
}

const IdpField: FC<IdpFieldProps> = ({
	name,
	fieldText,
	showStatusIndicator = false,
}) => {
	return (
		<span css={{ display: "flex", alignItems: "center", gap: "16px" }}>
			<p>{name}</p>
			{fieldText ? (
				<p css={styles.fieldText}>{fieldText}</p>
			) : (
				showStatusIndicator && (
					<div
						css={{
							display: "flex",
							alignItems: "center",
							gap: "8px",
							height: 0,
						}}
					>
						<StatusIndicator color="error" />
						<p>disabled</p>
					</div>
				)
			)}
		</span>
	);
};

interface TableRowCountProps {
	count: number;
	type: string;
}

const TableRowCount: FC<TableRowCountProps> = ({ count, type }) => {
	return (
		<div
			css={(theme) => ({
				margin: 0,
				fontSize: 13,
				color: theme.palette.text.secondary,
				"& strong": {
					color: theme.palette.text.primary,
				},
			})}
		>
			Showing <strong>{count}</strong> {type}
		</div>
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
												href={docs(
													`/admin/auth#${type.toLowerCase()}-sync-enterprise`,
												)}
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
	coderGroup: readonly string[];
}

const GroupRow: FC<GroupRowProps> = ({ idpGroup, coderGroup }) => {
	return (
		<TableRow data-testid={`group-${idpGroup}`}>
			<TableCell>{idpGroup}</TableCell>
			<TableCell>
				<PillList roles={coderGroup} />
			</TableCell>
		</TableRow>
	);
};

interface RoleRowProps {
	idpRole: string;
	coderRoles: readonly string[];
}

const RoleRow: FC<RoleRowProps> = ({ idpRole, coderRoles }) => {
	return (
		<TableRow data-testid={`role-${idpRole}`}>
			<TableCell>{idpRole}</TableCell>
			<TableCell>
				<PillList roles={coderRoles} />
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
	fieldText: (theme) => ({
		color: theme.palette.text.secondary,
		fontFamily: MONOSPACE_FONT_FAMILY,
	}),
	fields: () => ({
		marginBottom: 20,
		marginLeft: 16,
	}),
	tableInfo: () => ({
		marginBottom: 16,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default IdpSyncPageView;
