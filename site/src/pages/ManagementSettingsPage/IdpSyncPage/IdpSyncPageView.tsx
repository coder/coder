import type { Interpolation, Theme } from "@emotion/react";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
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
	Organization,
	RoleSyncSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Loader } from "components/Loader/Loader";
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
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpPillList } from "./IdpPillList";

interface IdpSyncPageViewProps {
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	groups: Group[] | undefined;
	groupsMap: Map<string, string>;
	organization: Organization;
	error?: unknown;
}

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	groupSyncSettings,
	roleSyncSettings,
	groups,
	groupsMap,
	organization,
	error,
}) => {
	const [searchParams] = useSearchParams();

	const getGroupNames = (groupIds: readonly string[]) => {
		return groupIds.map((groupId) => groupsMap.get(groupId) || groupId);
	};

	const tab = searchParams.get("tab") || "groups";

	const groupMappingCount = groupSyncSettings?.mapping
		? Object.entries(groupSyncSettings.mapping).length
		: 0;
	const legacyGroupMappingCount = groupSyncSettings?.legacy_group_name_mapping
		? Object.entries(groupSyncSettings.legacy_group_name_mapping).length
		: 0;
	const roleMappingCount = roleSyncSettings?.mapping
		? Object.entries(roleSyncSettings.mapping).length
		: 0;

	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (!groupSyncSettings || !roleSyncSettings || !groups) {
		return <Loader />;
	}

	return (
		<>
			<Stack spacing={2}>
				<Tabs active={tab}>
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
							<Stack direction="row" alignItems="center" spacing={6}>
								<IdpField
									name="Sync Field"
									fieldText={groupSyncSettings?.field}
									showDisabled
								/>
								<IdpField
									name="Regex Filter"
									fieldText={
										typeof groupSyncSettings?.regex_filter === "string"
											? groupSyncSettings.regex_filter
											: "none"
									}
								/>
								<IdpField
									name="Auto Create"
									fieldText={
										groupSyncSettings?.field
											? String(groupSyncSettings?.auto_create_missing_groups)
											: "n/a"
									}
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
							<ExportPolicyButton
								syncSettings={groupSyncSettings}
								organization={organization}
								type="groups"
							/>
						</Stack>
						<Stack spacing={6}>
							<IdpMappingTable type="Group" isEmpty={groupMappingCount === 0}>
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
							{groupSyncSettings?.legacy_group_name_mapping && (
								<section>
									<LegacyGroupSyncHeader />
									<IdpMappingTable
										type="Group"
										isEmpty={legacyGroupMappingCount === 0}
									>
										{Object.entries(groupSyncSettings.legacy_group_name_mapping)
											.sort()
											.map(([idpGroup, groupId]) => (
												<GroupRow
													key={idpGroup}
													idpGroup={idpGroup}
													coderGroup={getGroupNames([groupId])}
												/>
											))}
									</IdpMappingTable>
								</section>
							)}
						</Stack>
					</>
				) : (
					<>
						<div css={styles.fields}>
							<IdpField
								name="Sync Field"
								fieldText={roleSyncSettings?.field}
								showDisabled
							/>
						</div>
						<Stack
							direction="row"
							alignItems="baseline"
							justifyContent="space-between"
							css={styles.tableInfo}
						>
							<TableRowCount count={roleMappingCount} type="roles" />
							<ExportPolicyButton
								syncSettings={roleSyncSettings}
								organization={organization}
								type="roles"
							/>
						</Stack>
						<IdpMappingTable type="Role" isEmpty={roleMappingCount === 0}>
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
			</Stack>
		</>
	);
};

interface IdpFieldProps {
	name: string;
	fieldText: string | undefined;
	showDisabled?: boolean;
}

const IdpField: FC<IdpFieldProps> = ({
	name,
	fieldText,
	showDisabled = false,
}) => {
	return (
		<span
			css={{
				display: "flex",
				alignItems: "center",
				gap: "16px",
			}}
		>
			<p css={styles.fieldLabel}>{name}</p>
			{fieldText ? (
				<p css={styles.fieldText}>{fieldText}</p>
			) : (
				showDisabled && (
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
						<TableCell width="45%">IdP {type}</TableCell>
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
												href={docs("/admin/users/idp-sync")}
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
				<IdpPillList roles={coderGroup} />
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
				<IdpPillList roles={coderRoles} />
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

const LegacyGroupSyncHeader: FC = () => {
	return (
		<h4
			css={{
				fontSize: 20,
				fontWeight: 500,
			}}
		>
			<Stack direction="row" alignItems="end" spacing={1}>
				<span>Legacy Group Sync Settings</span>
				<HelpTooltip>
					<HelpTooltipTrigger />
					<HelpTooltipContent>
						<HelpTooltipTitle>Legacy Group Sync Settings</HelpTooltipTitle>
						<HelpTooltipText>
							These settings were configured using environment variables, and
							only apply to the default organization. It is now recommended to
							configure IdP sync via the CLI, which enables sync to be
							configured for any organization, and for those settings to be
							persisted without manually setting environment variables.{" "}
							<Link href={docs("/admin/users/idp-sync")}>
								Learn more&hellip;
							</Link>
						</HelpTooltipText>
					</HelpTooltipContent>
				</HelpTooltip>
			</Stack>
		</h4>
	);
};

const styles = {
	fieldText: {
		fontFamily: MONOSPACE_FONT_FAMILY,
		whiteSpace: "nowrap",
		paddingBottom: ".02rem",
	},
	fieldLabel: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	fields: () => ({
		marginLeft: 16,
		fontSize: 14,
	}),
	tableInfo: () => ({
		marginBottom: 16,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default IdpSyncPageView;
