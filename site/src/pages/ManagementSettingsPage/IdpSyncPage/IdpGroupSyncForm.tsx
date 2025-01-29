import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type {
	Group,
	GroupSyncSettings,
	Organization,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import { useFormik } from "formik";
import { Plus, Trash } from "lucide-react";
import { type FC, useId, useState } from "react";
import { docs } from "utils/docs";
import { isUUID } from "utils/uuid";
import * as Yup from "yup";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpMappingTable } from "./IdpMappingTable";
import { IdpPillList } from "./IdpPillList";

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
	mapping: Yup.object()
		.test(
			"valid-mapping",
			"Invalid group sync settings mapping structure",
			(value) => {
				if (!value) return true;
				return Object.entries(value).every(
					([key, arr]) =>
						typeof key === "string" &&
						Array.isArray(arr) &&
						arr.every((item) => {
							return typeof item === "string" && isUUID(item);
						}),
				);
			},
		)
		.default({}),
});

export const IdpGroupSyncForm = ({
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
	const id = useId();

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

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset
				disabled={form.isSubmitting}
				className="flex flex-col border-none gap-8 pt-2"
			>
				<div className="flex justify-end">
					<ExportPolicyButton
						syncSettings={groupSyncSettings}
						organization={organization}
						type="groups"
					/>
				</div>
				<div className="grid items-center gap-3">
					<div className="flex flex-row items-center gap-5">
						<div className="grid grid-cols-2 gap-2 grid-rows-[20px_auto_20px]">
							<Label className="text-sm" htmlFor={`${id}-sync-field`}>
								Group sync field
							</Label>
							<Label className="text-sm" htmlFor={`${id}-regex-filter`}>
								Regex filter
							</Label>
							<Input
								id={`${id}-sync-field`}
								value={form.values.field}
								onChange={(event) => {
									void form.setFieldValue("field", event.target.value);
								}}
								className="w-72"
							/>
							<div className="flex flex-row gap-2">
								<Input
									id={`${id}-regex-filter`}
									value={form.values.regex_filter ?? ""}
									onChange={(event) => {
										void form.setFieldValue("regex_filter", event.target.value);
									}}
									className="min-w-40"
								/>
								<Button
									type="submit"
									disabled={form.isSubmitting || !form.dirty}
									onClick={(event) => {
										event.preventDefault();
										form.handleSubmit();
									}}
								>
									<Spinner loading={form.isSubmitting} />
									Save
								</Button>
							</div>
							<p className="text-content-secondary text-2xs m-0">
								If empty, group sync is deactivated
							</p>
						</div>
					</div>
					{form.errors.field ||
						(form.errors.regex_filter && (
							<p className="text-content-danger text-sm m-0">
								{form.errors.field || form.errors.regex_filter}
							</p>
						))}
				</div>
				<div className="flex flex-row items-center gap-3">
					<Spinner size="sm" loading={form.isSubmitting} className="w-9">
						<Switch
							id={`${id}-auto-create-missing-groups`}
							checked={form.values.auto_create_missing_groups}
							onCheckedChange={(checked) => {
								void form.setFieldValue("auto_create_missing_groups", checked);
								form.handleSubmit();
							}}
						/>
					</Spinner>
					<span className="flex flex-row items-center gap-1">
						<Label htmlFor={`${id}-auto-create-missing-groups`}>
							Auto create missing groups
						</Label>
						<AutoCreateMissingGroupsHelpTooltip />
					</span>
				</div>
				<div className="flex flex-row gap-2 justify-between items-start">
					<div className="grid items-center gap-1">
						<Label className="text-sm" htmlFor={`${id}-idp-group-name`}>
							IdP group name
						</Label>
						<Input
							id={`${id}-idp-group-name`}
							value={idpGroupName}
							className="w-72"
							onChange={(event) => {
								setIdpGroupName(event.target.value);
							}}
						/>
					</div>
					<div className="grid items-center gap-1 flex-1">
						<Label className="text-sm" htmlFor={`${id}-coder-group`}>
							Coder group
						</Label>
						<MultiSelectCombobox
							inputProps={{
								id: `${id}-coder-group`,
							}}
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
						<div />
						<Button
							type="submit"
							className="min-w-fit"
							disabled={!idpGroupName || coderGroups.length === 0}
							onClick={() => {
								const newSyncSettings = {
									...form.values,
									mapping: {
										...form.values.mapping,
										[idpGroupName]: coderGroups.map((group) => group.value),
									},
								};
								void form.setFieldValue("mapping", newSyncSettings.mapping);
								form.handleSubmit();
								setIdpGroupName("");
								setCoderGroups([]);
							}}
						>
							<Spinner loading={form.isSubmitting}>
								<Plus size={14} />
							</Spinner>
							Add IdP group
						</Button>
					</div>
				</div>
				{form.errors.mapping && (
					<p className="text-content-danger text-sm m-0">
						{Object.values(form.errors.mapping || {})}
					</p>
				)}
				<div className="flex flex-col">
					<IdpMappingTable type="Group" rowCount={groupMappingCount}>
						{groupSyncSettings?.mapping &&
							Object.entries(groupSyncSettings.mapping)
								.sort(([a], [b]) =>
									a.toLowerCase().localeCompare(b.toLowerCase()),
								)
								.map(([idpGroup, groups]) => (
									<GroupRow
										key={idpGroup}
										idpGroup={idpGroup}
										coderGroup={getGroupNames(groups)}
										onDelete={handleDelete}
									/>
								))}
					</IdpMappingTable>

					{groupSyncSettings?.legacy_group_name_mapping && (
						<div>
							<LegacyGroupSyncHeader />
							<IdpMappingTable type="Group" rowCount={legacyGroupMappingCount}>
								{Object.entries(groupSyncSettings.legacy_group_name_mapping)
									.sort(([a], [b]) =>
										a.toLowerCase().localeCompare(b.toLowerCase()),
									)
									.map(([idpGroup, groupId]) => (
										<GroupRow
											key={groupId}
											idpGroup={idpGroup}
											coderGroup={getGroupNames([groupId])}
											onDelete={handleDelete}
										/>
									))}
							</IdpMappingTable>
						</div>
					)}
				</div>
			</fieldset>
		</form>
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
					size="icon"
					className="text-content-primary"
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

const AutoCreateMissingGroupsHelpTooltip: FC = () => {
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

const LegacyGroupSyncHeader: FC = () => {
	return (
		<h4 className="text-xl font-medium">
			<div className="flex items-end gap-2">
				<span>Legacy group sync settings</span>
				<HelpTooltip>
					<HelpTooltipTrigger />
					<HelpTooltipContent>
						<HelpTooltipTitle>Legacy group sync settings</HelpTooltipTitle>
						<HelpTooltipText>
							These settings were configured using environment variables, and
							only apply to the default organization. It is now recommended to
							configure IdP sync via the CLI or the UI, which enables sync to be
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
