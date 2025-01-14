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
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Loader } from "components/Loader/Loader";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Switch } from "components/Switch/Switch";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useFormik } from "formik";
import { Plus, SquareArrowOutUpRight, Trash } from "lucide-react";
import { type FC, useState } from "react";
import { useSearchParams } from "react-router-dom";
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
			<div className="flex flex-col gap-4">
				<Tabs active={tab}>
					<TabsList>
						<TabLink to="?tab=groups" value="groups">
							Group sync settings
						</TabLink>
						<TabLink to="?tab=roles" value="roles">
							Role sync settings
						</TabLink>
					</TabsList>
				</Tabs>
				{tab === "groups" ? (
					<IdpGroupSyncForm
						groupSyncSettings={groupSyncSettings}
						groupMappingCount={groupMappingCount}
						legacyGroupMappingCount={legacyGroupMappingCount}
						groups={groups}
						groupsMap={groupsMap}
						organization={organization}
						onSubmit={onSubmitGroupSyncSettings}
					/>
				) : (
					<IdpRoleSyncForm
						roleSyncSettings={roleSyncSettings}
						roleMappingCount={roleMappingCount}
						roles={roles || []}
						organization={organization}
						onSubmit={onSubmitRoleSyncSettings}
					/>
				)}
			</div>
		</>
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
						<TableCell width="10%" />
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
											<Button variant="outline" asChild>
												<a
													href={docs("/admin/users/idp-sync")}
													className="no-underline"
												>
													<SquareArrowOutUpRight size={14} />
													How to setup IdP {type} sync
												</a>
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
	onDelete: (idpOrg: string) => void;
}

const GroupRow: FC<GroupRowProps> = ({ idpGroup, coderGroup, onDelete }) => {
	return (
		<TableRow data-testid={`group-${idpGroup}`}>
			<TableCell>{idpGroup}</TableCell>
			<TableCell>
				<IdpPillList roles={coderGroup} />
			</TableCell>
			<TableCell>
				<Button
					variant="outline"
					className="w-8 h-8 px-1.5 py-1.5 text-content-secondary"
					aria-label="delete"
					onClick={() => onDelete(idpGroup)}
				>
					<Trash />
					<span className="sr-only">Delete IdP mapping</span>
				</Button>
			</TableCell>
		</TableRow>
	);
};

interface IdpGroupSyncFormProps {
	groupSyncSettings: GroupSyncSettings;
	groupsMap: Map<string, string>;
	groups: Group[];
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
	groups,
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
	const [idpGroupName, setIdpGroupName] = useState("");
	const [coderGroups, setCoderGroups] = useState<Option[]>([]);

	const getGroupNames = (groupIds: readonly string[]) => {
		return groupIds.map((groupId) => groupsMap.get(groupId) || groupId);
	};

	const handleDelete = async (idpOrg: string) => {
		const newMapping = Object.fromEntries(
			Object.entries(form.values.mapping || {}).filter(
				([key]) => key !== idpOrg,
			),
		);
		const newSyncSettings = {
			...form.values,
			mapping: newMapping,
		};
		void form.setFieldValue("mapping", newSyncSettings.mapping);
		form.handleSubmit();
	};

	const SYNC_FIELD_ID = "sync-field";
	const REGEX_FILTER_ID = "regex-filter";
	const AUTO_CREATE_MISSING_GROUPS_ID = "auto-create-missing-groups";
	const IDP_GROUP_NAME_ID = "idp-group-name";

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset
				disabled={form.isSubmitting}
				className="flex flex-col border-none gap-5"
			>
				<div className="grid items-center gap-3">
					<div className="flex flex-row items-center gap-5">
						<div className="grid grid-cols-2 gap-2 grid-rows-[20px_auto_20px] w-96">
							<Label className="text-sm" htmlFor={SYNC_FIELD_ID}>
								Group sync field
							</Label>
							<Label className="text-sm" htmlFor={SYNC_FIELD_ID}>
								Regex filter
							</Label>
							<Input
								id={SYNC_FIELD_ID}
								value={form.values.field}
								onChange={async (event) => {
									void form.setFieldValue("field", event.target.value);
								}}
								className="min-w-40"
							/>
							<div className="flex flex-row gap-2">
								<Input
									id={REGEX_FILTER_ID}
									value={form.values.regex_filter ?? ""}
									onChange={async (event) => {
										void form.setFieldValue("regex_filter", event.target.value);
									}}
									className="min-w-40"
								/>
								<Button
									className="w-20"
									type="submit"
									disabled={form.isSubmitting || !form.dirty}
									onClick={(event) => {
										event.preventDefault();
										form.handleSubmit();
									}}
								>
									Save
								</Button>
							</div>
							<p className="text-content-secondary text-2xs m-0">
								If empty, group sync is deactivated
							</p>
						</div>
					</div>

					<div className="flex flex-row items-center gap-3">
						<Switch
							id={AUTO_CREATE_MISSING_GROUPS_ID}
							checked={form.values.auto_create_missing_groups}
							onCheckedChange={async (checked) => {
								void form.setFieldValue("organization_assign_default", checked);
								form.handleSubmit();
							}}
						/>
						<span className="flex flex-row items-center gap-1">
							<Label htmlFor={AUTO_CREATE_MISSING_GROUPS_ID}>
								Auto Create Missing Groups
							</Label>
							<AutoCreateMissingGroupsHelpTooltip />
						</span>
					</div>
				</div>
				<div className="flex flex-col gap-4">
					<div className="flex flex-row pt-4 gap-2 justify-between items-start">
						<div className="grid items-center gap-1">
							<Label className="text-sm" htmlFor={IDP_GROUP_NAME_ID}>
								IdP group name
							</Label>
							<Input
								id={IDP_GROUP_NAME_ID}
								value={idpGroupName}
								className="min-w-72 w-72"
								onChange={(event) => {
									setIdpGroupName(event.target.value);
								}}
							/>
						</div>
						<div className="grid items-center gap-1 flex-1">
							<Label className="text-sm" htmlFor=":r1d:">
								Coder group
							</Label>
							<MultiSelectCombobox
								className="min-w-60 max-w-3xl"
								value={coderGroups}
								onChange={setCoderGroups}
								defaultOptions={groups.map((group) => ({
									label: group.display_name || group.name,
									value: group.id,
								}))}
								hidePlaceholderWhenSelected
								placeholder="Select group"
								emptyIndicator={
									<p className="text-center text-md text-content-primary">
										All groups selected
									</p>
								}
							/>
						</div>
						<div className="grid grid-rows-[28px_auto]">
							&nbsp;
							<Button
								className="mb-px"
								type="submit"
								disabled={!idpGroupName || coderGroups.length === 0}
								onClick={async () => {
									const newSyncSettings = {
										...form.values,
										mapping: {
											...form.values.mapping,
											[idpGroupName]: coderGroups.map((role) => role.value),
										},
									};
									void form.setFieldValue("mapping", newSyncSettings.mapping);
									form.handleSubmit();
									setIdpGroupName("");
									setCoderGroups([]);
								}}
							>
								<Plus size={14} />
								Add IdP group
							</Button>
						</div>
					</div>
				</div>
				<div className="flex gap-12">
					<div className="flex flex-col w-full">
						<IdpMappingTable type="Group" isEmpty={groupMappingCount === 0}>
							{groupSyncSettings?.mapping &&
								Object.entries(groupSyncSettings.mapping)
									.sort()
									.map(([idpGroup, groups]) => (
										<GroupRow
											key={idpGroup}
											idpGroup={idpGroup}
											coderGroup={getGroupNames(groups)}
											onDelete={handleDelete}
										/>
									))}
						</IdpMappingTable>
						<div className="flex justify-between">
							<span className="pt-2">
								<ExportPolicyButton
									syncSettings={groupSyncSettings}
									organization={organization}
									type="groups"
								/>
							</span>
							<TableRowCount count={groupMappingCount} type="groups" />
						</div>
					</div>
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
											onDelete={handleDelete}
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
	roles: Role[];
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
	roles,
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
	const [idpRoleName, setIdpRoleName] = useState("");
	const [coderRoles, setCoderRoles] = useState<Option[]>([]);

	const handleDelete = async (idpOrg: string) => {
		const newMapping = Object.fromEntries(
			Object.entries(form.values.mapping || {}).filter(
				([key]) => key !== idpOrg,
			),
		);
		const newSyncSettings = {
			...form.values,
			mapping: newMapping,
		};
		void form.setFieldValue("mapping", newSyncSettings.mapping);
		form.handleSubmit();
	};

	const SYNC_FIELD_ID = "sync-field";
	const IDP_ROLE_NAME_ID = "idp-role-name";

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset
				disabled={form.isSubmitting}
				className="flex flex-col border-none gap-3"
			>
				<div className="grid items-center gap-1">
					<Label className="text-sm" htmlFor={SYNC_FIELD_ID}>
						Role sync field
					</Label>
					<div className="flex flex-row items-center gap-5">
						<div className="flex flex-row gap-2 w-72">
							<Input
								id={SYNC_FIELD_ID}
								value={form.values.field}
								onChange={async (event) => {
									void form.setFieldValue("field", event.target.value);
								}}
							/>
							<Button
								className="w-20"
								type="submit"
								disabled={form.isSubmitting || !form.dirty}
								onClick={(event) => {
									event.preventDefault();
									form.handleSubmit();
								}}
							>
								Save
							</Button>
						</div>
					</div>
					<p className="text-content-secondary text-2xs m-0">
						If empty, role sync is deactivated
					</p>
				</div>
				<div className="flex flex-col gap-4">
					<div className="flex flex-row pt-4 gap-2 justify-between items-start">
						<div className="grid items-center gap-1">
							<Label className="text-sm" htmlFor={IDP_ROLE_NAME_ID}>
								IdP role name
							</Label>
							<Input
								id={IDP_ROLE_NAME_ID}
								value={idpRoleName}
								className="min-w-72 w-72"
								onChange={(event) => {
									setIdpRoleName(event.target.value);
								}}
							/>
						</div>
						<div className="grid items-center gap-1 flex-1">
							<Label className="text-sm" htmlFor=":r1d:">
								Coder role
							</Label>
							<MultiSelectCombobox
								className="min-w-60 max-w-3xl"
								value={coderRoles}
								onChange={setCoderRoles}
								defaultOptions={roles.map((role) => ({
									label: role.display_name || role.name,
									value: role.name,
								}))}
								hidePlaceholderWhenSelected
								placeholder="Select role"
								emptyIndicator={
									<p className="text-center text-md text-content-primary">
										All roles selected
									</p>
								}
							/>
						</div>
						<div className="grid grid-rows-[28px_auto]">
							&nbsp;
							<Button
								className="mb-px"
								type="submit"
								disabled={!idpRoleName || coderRoles.length === 0}
								onClick={async () => {
									const newSyncSettings = {
										...form.values,
										mapping: {
											...form.values.mapping,
											[idpRoleName]: coderRoles.map((role) => role.value),
										},
									};
									void form.setFieldValue("mapping", newSyncSettings.mapping);
									form.handleSubmit();
									setIdpRoleName("");
									setCoderRoles([]);
								}}
							>
								<Plus size={14} />
								Add IdP role
							</Button>
						</div>
					</div>
					<div>
						<IdpMappingTable type="Role" isEmpty={roleMappingCount === 0}>
							{roleSyncSettings?.mapping &&
								Object.entries(roleSyncSettings.mapping)
									.sort()
									.map(([idpRole, roles]) => (
										<RoleRow
											key={idpRole}
											idpRole={idpRole}
											coderRoles={roles}
											onDelete={handleDelete}
										/>
									))}
						</IdpMappingTable>
						<div className="flex justify-between">
							<span className="pt-2">
								<ExportPolicyButton
									syncSettings={roleSyncSettings}
									organization={organization}
									type="roles"
								/>
							</span>
							<TableRowCount count={roleMappingCount} type="roles" />
						</div>
					</div>
				</div>
			</fieldset>
		</form>
	);
};

interface RoleRowProps {
	idpRole: string;
	coderRoles: readonly string[];
	onDelete: (idpOrg: string) => void;
}

const RoleRow: FC<RoleRowProps> = ({ idpRole, coderRoles, onDelete }) => {
	return (
		<TableRow data-testid={`role-${idpRole}`}>
			<TableCell>{idpRole}</TableCell>
			<TableCell>
				<IdpPillList roles={coderRoles} />
			</TableCell>
			<TableCell>
				<Button
					variant="outline"
					className="w-8 h-8 px-1.5 py-1.5 text-content-secondary"
					aria-label="delete"
					onClick={() => onDelete(idpRole)}
				>
					<Trash />
					<span className="sr-only">Delete IdP mapping</span>
				</Button>
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

export const AutoCreateMissingGroupsHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger />
			<HelpTooltipContent>
				<HelpTooltipText>
					Enabling auto create missing groups will automatically create groups
					returned by the OIDC provider if they do not exist in Coder.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

export default IdpSyncPageView;
