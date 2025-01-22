import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { Organization, Role, RoleSyncSettings } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import { Plus, Trash } from "lucide-react";
import { type FC, useState } from "react";
import * as Yup from "yup";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpMappingTable } from "./IdpMappingTable";
import { IdpPillList } from "./IdpPillList";

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

export const IdpRoleSyncForm = ({
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
	const CODER_ROLE_ID = "coder-role";

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
								className="min-w-72 w-72"
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
				<div className="flex flex-row gap-2 justify-between items-start">
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
						<Label className="text-sm" htmlFor={CODER_ROLE_ID}>
							Coder role
						</Label>
						<MultiSelectCombobox
							inputProps={{
								id: CODER_ROLE_ID,
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
				<IdpMappingTable type="Role" rowCount={roleMappingCount}>
					{roleSyncSettings?.mapping &&
						Object.entries(roleSyncSettings.mapping)
							.sort(([a], [b]) => a.toLowerCase().localeCompare(b.toLowerCase()))
							.map(([idpRole, roles]) => (
								<RoleRow
									key={idpRole}
									idpRole={idpRole}
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
