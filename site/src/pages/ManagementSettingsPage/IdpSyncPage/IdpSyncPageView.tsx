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
import type { FC } from "react";
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
	// const theme = useTheme();

	const groupsMap = new Map<string, string>();
	if (groups) {
		for (const group of groups) {
			groupsMap.set(group.id, group.display_name || group.name);
		}
	}

	const getGroupNames = (groupIds: readonly string[]) => {
		return groupIds.map((groupId) => groupsMap.get(groupId) || groupId);
	};

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
					<Stack spacing={2} css={styles.fields}>
						{/* Semantically fieldset is used for forms. In the future this screen will allow
						 updates to these fields in a form */}
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Groups</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={8}>
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
						</fieldset>
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Roles</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={3}>
								<IdpField
									name={"Sync Field"}
									fieldText={roleSyncSettings?.field}
									showStatusIndicator
								/>
							</Stack>
						</fieldset>
					</Stack>
					<Stack spacing={6}>
						<IdpMappingTable
							type="Group"
							isEmpty={Boolean(
								!groupSyncSettings?.mapping ||
									Object.entries(groupSyncSettings?.mapping).length === 0,
							)}
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
						<IdpMappingTable
							type="Role"
							isEmpty={Boolean(
								!roleSyncSettings?.mapping ||
									Object.entries(roleSyncSettings?.mapping).length === 0,
							)}
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
					</Stack>
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
			<h4>{name}</h4>
			<p css={styles.field}>
				{fieldText ||
					(showStatusIndicator && (
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
					))}
			</p>
		</span>
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
	field: (theme) => ({
		color: theme.palette.text.secondary,
		fontFamily: MONOSPACE_FONT_FAMILY,
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
