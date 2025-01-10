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
	Role,
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
import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useFormik } from "formik";
import type { FC } from "react";
import { useSearchParams } from "react-router-dom";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { docs } from "utils/docs";
import * as Yup from "yup";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpPillList } from "./IdpPillList";

interface IdpSyncPageViewProps {
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	groups: Group[] | undefined;
	groupsMap: Map<string, string>;
	roles: Role[] | undefined;
	organization: Organization;
	error?: unknown;
	onSubmitGroupSyncSettings: (data: GroupSyncSettings) => void;
	onSubmitRoleSyncSettings: (data: RoleSyncSettings) => void;
}

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	groupSyncSettings,
	roleSyncSettings,
	groups,
	groupsMap,
	roles,
	organization,
	error,
	onSubmitGroupSyncSettings,
	onSubmitRoleSyncSettings,
}) => {
	const [searchParams] = useSearchParams();
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
			<div className="gap-4">
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
					<IdpGroupSyncForm
						groupSyncSettings={groupSyncSettings}
						groupMappingCount={groupMappingCount}
						legacyGroupMappingCount={legacyGroupMappingCount}
						groupsMap={groupsMap}
						organization={organization}
						onSubmit={onSubmitGroupSyncSettings}
					/>
				) : (
					<IdpRoleSyncForm
						roleSyncSettings={roleSyncSettings}
						roleMappingCount={roleMappingCount}
						organization={organization}
						onSubmit={onSubmitRoleSyncSettings}
					/>
				)}
			</div>
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

interface IdpGroupSyncFormProps {
	groupSyncSettings: GroupSyncSettings;
	groupsMap: Map<string, string>;
	groupMappingCount: number;
	legacyGroupMappingCount: number;
	organization: Organization;
	onSubmit: (data: GroupSyncSettings) => void;
}

const groupSyncValidationSchema = Yup.object({
	field: Yup.string().trim(),
	regex_filter: Yup.string().trim(),
	auto_create_missing_groups: Yup.boolean(),
	mapping: Yup.object().shape({
		[`${String}`]: Yup.array().of(Yup.string()),
	}),
});

const IdpGroupSyncForm = ({
	groupSyncSettings,
	groupMappingCount,
	legacyGroupMappingCount,
	groupsMap,
	organization,
	onSubmit,
}: IdpGroupSyncFormProps) => {
	const form = useFormik<GroupSyncSettings>({
		initialValues: {
			field: groupSyncSettings?.field ?? "",
			regex_filter: groupSyncSettings?.regex_filter ?? "",
			auto_create_missing_groups:
				groupSyncSettings?.auto_create_missing_groups ?? false,
			mapping: groupSyncSettings?.mapping ?? {},
		},
		validationSchema: groupSyncValidationSchema,
		onSubmit,
		enableReinitialize: Boolean(groupSyncSettings),
	});

	const getGroupNames = (groupIds: readonly string[]) => {
		return groupIds.map((groupId) => groupsMap.get(groupId) || groupId);
	};

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset disabled={form.isSubmitting} className="border-none">
				<div css={styles.fields}>
					<div className="flex items-center gap-12">
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
					</div>
				</div>
				<div className="flex items-baseline justify-between mb-4">
					<TableRowCount count={groupMappingCount} type="groups" />
					<ExportPolicyButton
						syncSettings={groupSyncSettings}
						organization={organization}
						type="groups"
					/>
				</div>
				<div className="flex gap-12">
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
				</div>
			</fieldset>
		</form>
	);
};

interface IdpRoleSyncFormProps {
	roleSyncSettings: RoleSyncSettings;
	roleMappingCount: number;
	organization: Organization;
	onSubmit: (data: RoleSyncSettings) => void;
}

const roleyncValidationSchema = Yup.object({
	field: Yup.string().trim(),
	regex_filter: Yup.string().trim(),
	auto_create_missing_groups: Yup.boolean(),
	mapping: Yup.object().shape({
		[`${String}`]: Yup.array().of(Yup.string()),
	}),
});

const IdpRoleSyncForm = ({
	roleSyncSettings,
	roleMappingCount,
	organization,
	onSubmit,
}: IdpRoleSyncFormProps) => {
	const form = useFormik<RoleSyncSettings>({
		initialValues: {
			field: roleSyncSettings?.field ?? "",
			mapping: roleSyncSettings?.mapping ?? {},
		},
		validationSchema: roleyncValidationSchema,
		onSubmit,
		enableReinitialize: Boolean(roleSyncSettings),
	});

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset disabled={form.isSubmitting} className="border-none">
				<div css={styles.fields}>
					<IdpField
						name="Sync Field"
						fieldText={roleSyncSettings?.field}
						showDisabled
					/>
				</div>
				<div className="flex items-baseline justify-between mb-4">
					<TableRowCount count={roleMappingCount} type="roles" />
					<ExportPolicyButton
						syncSettings={roleSyncSettings}
						organization={organization}
						type="roles"
					/>
				</div>
				<IdpMappingTable type="Role" isEmpty={roleMappingCount === 0}>
					{roleSyncSettings?.mapping &&
						Object.entries(roleSyncSettings.mapping)
							.sort()
							.map(([idpRole, roles]) => (
								<RoleRow key={idpRole} idpRole={idpRole} coderRoles={roles} />
							))}
				</IdpMappingTable>
			</fieldset>
		</form>
	);
};

interface GroupRowProps {
	idpGroup: string;
	coderGroup: readonly string[];
}

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
			<div className="flex items-end gap-2">
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
			</div>
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
