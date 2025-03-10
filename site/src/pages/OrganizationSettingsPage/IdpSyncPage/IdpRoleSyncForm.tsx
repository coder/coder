import type { Organization, Role, RoleSyncSettings } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Combobox } from "components/Combobox/Combobox";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "components/Spinner/Spinner";
import { TableCell, TableRow } from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useFormik } from "formik";
import { Plus, Trash, TriangleAlert } from "lucide-react";
import { type FC, type KeyboardEventHandler, useId, useState } from "react";
import * as Yup from "yup";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpMappingTable } from "./IdpMappingTable";
import { IdpPillList } from "./IdpPillList";

const roleSyncValidationSchema = Yup.object({
	field: Yup.string().trim(),
	regex_filter: Yup.string().trim(),
	auto_create_missing_groups: Yup.boolean(),
	mapping: Yup.object()
		.test(
			"valid-mapping",
			"Invalid role sync settings mapping structure",
			(value) => {
				if (!value) return true;
				return Object.entries(value).every(
					([key, arr]) =>
						typeof key === "string" &&
						Array.isArray(arr) &&
						arr.every((item) => {
							return typeof item === "string";
						}),
				);
			},
		)
		.default({}),
});

interface IdpRoleSyncFormProps {
	roleSyncSettings: RoleSyncSettings;
	claimFieldValues: readonly string[] | undefined;
	roleMappingCount: number;
	organization: Organization;
	roles: Role[];
	onSubmit: (data: RoleSyncSettings) => void;
	onSyncFieldChange: (value: string) => void;
}

export const IdpRoleSyncForm: FC<IdpRoleSyncFormProps> = ({
	roleSyncSettings,
	claimFieldValues,
	roleMappingCount,
	organization,
	roles,
	onSubmit,
	onSyncFieldChange,
}) => {
	const form = useFormik<RoleSyncSettings>({
		initialValues: {
			field: roleSyncSettings?.field ?? "",
			mapping: roleSyncSettings?.mapping ?? {},
		},
		validationSchema: roleSyncValidationSchema,
		onSubmit,
		enableReinitialize: Boolean(roleSyncSettings),
	});
	const [idpRoleName, setIdpRoleName] = useState("");
	const [coderRoles, setCoderRoles] = useState<Option[]>([]);
	const id = useId();
	const [comboInputValue, setComboInputValue] = useState("");
	const [open, setOpen] = useState(false);

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
			setIdpRoleName(comboInputValue);
			setComboInputValue("");
			setOpen(false);
		}
	};

	return (
		<form onSubmit={form.handleSubmit}>
			<fieldset
				disabled={form.isSubmitting}
				className="flex flex-col border-none gap-8 pt-2"
			>
				<div className="flex justify-end">
					<ExportPolicyButton
						syncSettings={roleSyncSettings}
						organization={organization}
						type="roles"
					/>
				</div>
				<div className="grid items-center gap-1">
					<Label className="text-sm" htmlFor={`${id}-sync-field`}>
						Role sync field
					</Label>
					<div className="flex flex-row items-center gap-5">
						<div className="flex flex-row gap-2">
							<Input
								id={`${id}-sync-field`}
								value={form.values.field}
								onChange={(event) => {
									void form.setFieldValue("field", event.target.value);
									onSyncFieldChange(event.target.value);
								}}
								className="w-72"
							/>
							<Button
								className="px-6"
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
					</div>
					<p className="text-content-secondary text-2xs m-0">
						If empty, role sync is deactivated
					</p>
				</div>
				{form.errors.field && (
					<p className="text-content-danger text-sm m-0">{form.errors.field}</p>
				)}
				<div className="flex flex-row gap-2 justify-between items-start">
					<div className="grid items-center gap-1">
						<Label className="text-sm" htmlFor={`${id}-idp-role-name`}>
							IdP role name
						</Label>
						{claimFieldValues ? (
							<Combobox
								value={idpRoleName}
								options={claimFieldValues}
								placeholder="Select IdP role"
								open={open}
								onOpenChange={setOpen}
								inputValue={comboInputValue}
								onInputChange={setComboInputValue}
								onKeyDown={handleKeyDown}
								onSelect={(value) => {
									setIdpRoleName(value);
									setOpen(false);
								}}
							/>
						) : (
							<Input
								id={`${id}-idp-role-name`}
								value={idpRoleName}
								className="w-72"
								onChange={(event) => {
									setIdpRoleName(event.target.value);
								}}
							/>
						)}
					</div>
					<div className="grid items-center gap-1 flex-1">
						<Label className="text-sm" htmlFor={`${id}-coder-role`}>
							Coder role
						</Label>
						<MultiSelectCombobox
							inputProps={{
								id: `${id}-coder-role`,
							}}
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
						<div />
						<Button
							type="submit"
							className="min-w-fit"
							disabled={!idpRoleName || coderRoles.length === 0}
							onClick={() => {
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
							<Spinner loading={form.isSubmitting}>
								<Plus size={14} />
							</Spinner>
							Add IdP role
						</Button>
					</div>
				</div>
				{form.errors.mapping && (
					<p className="text-content-danger text-sm m-0">
						{Object.values(form.errors.mapping || {})}
					</p>
				)}
				<IdpMappingTable type="Role" rowCount={roleMappingCount}>
					{roleSyncSettings?.mapping &&
						Object.entries(roleSyncSettings.mapping)
							.sort(([a], [b]) =>
								a.toLowerCase().localeCompare(b.toLowerCase()),
							)
							.map(([idpRole, roles]) => (
								<RoleRow
									key={idpRole}
									idpRole={idpRole}
									exists={claimFieldValues?.includes(idpRole)}
									coderRoles={roles}
									onDelete={handleDelete}
								/>
							))}
				</IdpMappingTable>
			</fieldset>
		</form>
	);
};

interface RoleRowProps {
	idpRole: string;
	exists: boolean | undefined;
	coderRoles: readonly string[];
	onDelete: (idpOrg: string) => void;
}

const RoleRow: FC<RoleRowProps> = ({
	idpRole,
	exists = true,
	coderRoles,
	onDelete,
}) => {
	return (
		<TableRow data-testid={`role-${idpRole}`}>
			<TableCell>
				<div className="flex flex-row items-center gap-2 text-content-primary">
					{idpRole}
					{!exists && (
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<TriangleAlert className="size-icon-xs cursor-pointer text-content-warning" />
								</TooltipTrigger>
								<TooltipContent
									align="start"
									alignOffset={-8}
									sideOffset={8}
									className="p-2 text-xs text-content-secondary max-w-sm"
								>
									This value has not be seen in the specified claim field
									before. You might want to check your IdP configuration and
									ensure that this value is not misspelled.
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
				</div>
			</TableCell>

			<TableCell>
				<IdpPillList roles={coderRoles} />
			</TableCell>

			<TableCell>
				<Button
					variant="outline"
					size="icon"
					className="text-content-primary"
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
