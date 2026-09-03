import { useFormik } from "formik";
import { PlusIcon, TrashIcon } from "lucide-react";
import { type FC, type KeyboardEventHandler, useId, useState } from "react";
import * as Yup from "yup";
import type { Group, GroupSyncSettings } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	MultiSelectCombobox,
	type Option,
} from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StackLabel,
	StackLabelHelperText,
} from "#/components/StackLabel/StackLabel";
import { Switch } from "#/components/Switch/Switch";
import { TableCell, TableRow } from "#/components/Table/Table";
import { isEveryoneGroup } from "#/modules/groups";
import { IdpUnseenClaimWarning } from "#/modules/idpSync/IdpUnseenClaimWarning";
import { docs } from "#/utils/docs";
import { isUUID } from "#/utils/uuid";
import { IdpMappingTable } from "./IdpMappingTable";
import { IdpPillList } from "./IdpPillList";

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

interface IdpGroupSyncFormProps {
	groupSyncSettings: GroupSyncSettings;
	claimFieldValues: readonly string[] | undefined;
	groupsMap: Map<string, string>;
	groups: Group[];
	groupMappingCount: number;
	legacyGroupMappingCount: number;
	onSubmit: (data: GroupSyncSettings) => void;
	onSyncFieldChange: (value: string) => void;
}

export const IdpGroupSyncForm: FC<IdpGroupSyncFormProps> = ({
	groupSyncSettings,
	claimFieldValues,
	groupMappingCount,
	legacyGroupMappingCount,
	groups,
	groupsMap,
	onSubmit,
	onSyncFieldChange,
}) => {
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
	const [comboInputValue, setComboInputValue] = useState("");
	const [open, setOpen] = useState(false);

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

	const handleKeyDown: KeyboardEventHandler<HTMLInputElement> = (event) => {
		if (
			event.key === "Enter" &&
			comboInputValue &&
			!claimFieldValues?.some(
				(value) => value === comboInputValue.toLowerCase(),
			)
		) {
			event.preventDefault();
			setIdpGroupName(comboInputValue);
			setComboInputValue("");
			setOpen(false);
		}
	};

	return (
		<form aria-label="Group sync" onSubmit={form.handleSubmit}>
			<fieldset
				disabled={form.isSubmitting}
				className="flex flex-col border-none gap-12"
			>
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							Sync field
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							If empty, group sync is deactivated.
						</SettingsHeaderDescription>
					</SettingsHeader>
					<div className="flex flex-col gap-6">
						<div className="flex flex-col gap-2">
							<div className="flex flex-row items-end gap-2">
								<div className="flex flex-col gap-2">
									<Label htmlFor={`${id}-sync-field`}>Group sync field</Label>
									<Input
										id={`${id}-sync-field`}
										value={form.values.field}
										onChange={(event) => {
											void form.setFieldValue("field", event.target.value);
											onSyncFieldChange(event.target.value);
										}}
										className="w-72"
									/>
								</div>
								<div className="flex flex-col gap-2">
									<Label htmlFor={`${id}-regex-filter`}>Regex filter</Label>
									<Input
										id={`${id}-regex-filter`}
										value={form.values.regex_filter ?? ""}
										onChange={(event) => {
											void form.setFieldValue(
												"regex_filter",
												event.target.value,
											);
										}}
										className="min-w-40"
									/>
								</div>
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
							{(form.errors.field || form.errors.regex_filter) && (
								<p className="text-content-destructive text-sm m-0">
									{form.errors.field || form.errors.regex_filter}
								</p>
							)}
						</div>
						<div className="flex items-start">
							<Spinner size="sm" loading={form.isSubmitting}>
								<Switch
									id={`${id}-auto-create-missing-groups`}
									checked={form.values.auto_create_missing_groups}
									onCheckedChange={(checked) => {
										void form.setFieldValue(
											"auto_create_missing_groups",
											checked,
										);
										form.handleSubmit();
									}}
								/>
							</Spinner>
							<Label htmlFor={`${id}-auto-create-missing-groups`}>
								<StackLabel>
									Auto create missing groups
									<StackLabelHelperText>
										Create groups from the IdP when they do not already exist in
										Coder.
									</StackLabelHelperText>
								</StackLabel>
							</Label>
						</div>
					</div>
				</div>
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							Group mapping
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Map IdP groups to Coder groups.
						</SettingsHeaderDescription>
					</SettingsHeader>
					<div className="flex flex-row gap-2 justify-between items-start">
						<div className="grid items-center gap-1 w-72">
							<Label className="text-sm" htmlFor={`${id}-idp-group-name`}>
								IdP group name
							</Label>
							{claimFieldValues ? (
								<Combobox
									open={open}
									onOpenChange={setOpen}
									value={idpGroupName}
									onValueChange={(value) => setIdpGroupName(value ?? "")}
								>
									<ComboboxTrigger asChild>
										<ComboboxButton
											className="w-72"
											selectedOption={
												idpGroupName
													? { label: idpGroupName, value: idpGroupName }
													: undefined
											}
											placeholder="Select IdP group"
										/>
									</ComboboxTrigger>
									<ComboboxContent className="w-72">
										<ComboboxInput
											value={comboInputValue}
											onValueChange={setComboInputValue}
											placeholder="Search..."
											onKeyDown={handleKeyDown}
										/>
										<ComboboxList>
											{claimFieldValues
												.filter((value) =>
													value
														.toLowerCase()
														.includes(comboInputValue.toLowerCase()),
												)
												.map((value) => (
													<ComboboxItem
														key={value}
														value={value}
														onSelect={() => setComboInputValue("")}
													>
														{value}
													</ComboboxItem>
												))}
										</ComboboxList>
									</ComboboxContent>
								</Combobox>
							) : (
								<Input
									id={`${id}-idp-group-name`}
									value={idpGroupName}
									className="w-72"
									onChange={(event) => {
										setIdpGroupName(event.target.value);
									}}
								/>
							)}
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
								options={groups
									.filter((group) => !isEveryoneGroup(group))
									.map((group) => ({
										label: group.display_name || group.name,
										value: group.id,
									}))}
								hidePlaceholderWhenSelected
								placeholder="Select group"
								emptyIndicator={
									<p className="text-center text-md text-content-primary">
										No more groups to select
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
									<PlusIcon />
								</Spinner>
								Add IdP group
							</Button>
						</div>
					</div>
					{form.errors.mapping && (
						<p className="text-content-destructive text-sm m-0">
							{Object.values(form.errors.mapping || {})}
						</p>
					)}
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
										exists={claimFieldValues?.includes(idpGroup)}
										coderGroup={getGroupNames(groups)}
										onDelete={handleDelete}
									/>
								))}
					</IdpMappingTable>
				</div>
				{groupSyncSettings?.legacy_group_name_mapping && (
					<div>
						<SettingsHeader>
							<SettingsHeaderTitle level="h2" hierarchy="secondary">
								Legacy group sync
							</SettingsHeaderTitle>
							<SettingsHeaderDescription>
								These settings were configured using environment variables, and
								only apply to the default organization. Configure IdP sync in
								the UI or CLI so it can apply to any organization.{" "}
								<SettingsHeaderDocsLink href={docs("/admin/users/idp-sync")} />
							</SettingsHeaderDescription>
						</SettingsHeader>
						<IdpMappingTable type="Group" rowCount={legacyGroupMappingCount}>
							{Object.entries(groupSyncSettings.legacy_group_name_mapping)
								.sort(([a], [b]) =>
									a.toLowerCase().localeCompare(b.toLowerCase()),
								)
								.map(([idpGroup, groupId]) => (
									<GroupRow
										key={groupId}
										idpGroup={idpGroup}
										exists={claimFieldValues?.includes(idpGroup)}
										coderGroup={getGroupNames([groupId])}
										onDelete={handleDelete}
									/>
								))}
						</IdpMappingTable>
					</div>
				)}
			</fieldset>
		</form>
	);
};

interface GroupRowProps {
	idpGroup: string;
	exists: boolean | undefined;
	coderGroup: readonly string[];
	onDelete: (idpOrg: string) => void;
}

const GroupRow: FC<GroupRowProps> = ({
	idpGroup,
	exists = true,
	coderGroup,
	onDelete,
}) => {
	return (
		<TableRow data-testid={`group-${idpGroup}`}>
			<TableCell>
				<div className="flex flex-row items-center gap-2 text-content-primary">
					{idpGroup}
					{!exists && <IdpUnseenClaimWarning />}
				</div>
			</TableCell>

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
					<TrashIcon />
					<span className="sr-only">Delete IdP mapping</span>
				</Button>
			</TableCell>
		</TableRow>
	);
};
