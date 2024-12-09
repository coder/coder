import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type {
	Organization,
	OrganizationSyncSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Switch } from "components/Switch/Switch";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useFormik } from "formik";
import { Plus, SquareArrowOutUpRight, Trash } from "lucide-react";
import { type FC, useState } from "react";
import { docs } from "utils/docs";
import * as Yup from "yup";
import { OrganizationPills } from "./OrganizationPills";

interface IdpSyncPageViewProps {
	organizationSyncSettings: OrganizationSyncSettings | undefined;
	organizations: readonly Organization[];
	onSubmit: (data: OrganizationSyncSettings) => void;
	error?: unknown;
}

const validationSchema = Yup.object({
	field: Yup.string().trim(),
	organization_assign_default: Yup.boolean(),
	mapping: Yup.object().shape({
		[`${String}`]: Yup.array().of(Yup.string()),
	}),
});

export const IdpOrgSyncPageView: FC<IdpSyncPageViewProps> = ({
	organizationSyncSettings,
	organizations,
	onSubmit,
	error,
}) => {
	const form = useFormik<OrganizationSyncSettings>({
		initialValues: {
			field: organizationSyncSettings?.field ?? "",
			organization_assign_default:
				organizationSyncSettings?.organization_assign_default ?? true,
			mapping: organizationSyncSettings?.mapping ?? {},
		},
		validationSchema: validationSchema,
		onSubmit,
		enableReinitialize: Boolean(organizationSyncSettings),
	});
	const [coderOrgs, setCoderOrgs] = useState<Option[]>([]);
	const [idpOrgName, setIdpOrgName] = useState("");
	const organizationMappingCount = form.values.mapping
		? Object.entries(form.values.mapping).length
		: 0;

	const getOrgNames = (orgIds: readonly string[]) => {
		return orgIds.map(
			(orgId) =>
				organizations.find((org) => org.id === orgId)?.display_name || orgId,
		);
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
	const ORGANIZATION_ASSIGN_DEFAULT_ID = "organization-assign-default";
	const IDP_ORGANIZATION_NAME_ID = "idp-organization-name";

	return (
		<div className="flex flex-col gap-2">
			{Boolean(error) && <ErrorAlert error={error} />}
			<form onSubmit={form.handleSubmit}>
				<fieldset disabled={form.isSubmitting} className="border-none">
					<div className="flex flex-row">
						<div className="grid items-center gap-1">
							<Label className="text-sm" htmlFor={SYNC_FIELD_ID}>
								Organization sync field
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
								<div className="flex flex-row items-center gap-3">
									<Switch
										id={ORGANIZATION_ASSIGN_DEFAULT_ID}
										checked={form.values.organization_assign_default}
										onCheckedChange={async (checked) => {
											void form.setFieldValue(
												"organization_assign_default",
												checked,
											);
											form.handleSubmit();
										}}
									/>
									<span className="flex flex-row items-center gap-1">
										<Label htmlFor={ORGANIZATION_ASSIGN_DEFAULT_ID}>
											Assign Default Organization
										</Label>
										<AssignDefaultOrgHelpTooltip />
									</span>
								</div>
							</div>
							<p className="text-content-secondary text-2xs m-0">
								If empty, organization sync is deactivated
							</p>
						</div>
					</div>

					<div className="flex flex-col gap-4">
						<div className="flex flex-row pt-8 gap-2 justify-between items-start">
							<div className="grid items-center gap-1">
								<Label className="text-sm" htmlFor={IDP_ORGANIZATION_NAME_ID}>
									IdP organization name
								</Label>
								<Input
									id={IDP_ORGANIZATION_NAME_ID}
									value={idpOrgName}
									className="min-w-72 w-72"
									onChange={(event) => {
										setIdpOrgName(event.target.value);
									}}
								/>
							</div>
							<div className="grid items-center gap-1 flex-1">
								<Label className="text-sm" htmlFor=":r1d:">
									Coder organization
								</Label>
								<MultiSelectCombobox
									className="min-w-60 max-w-3xl"
									value={coderOrgs}
									onChange={setCoderOrgs}
									defaultOptions={organizations.map((org) => ({
										label: org.display_name,
										value: org.id,
									}))}
									hidePlaceholderWhenSelected
									placeholder="Select organization"
									emptyIndicator={
										<p className="text-center text-md text-content-primary">
											All organizations selected
										</p>
									}
								/>
							</div>
							<div className="grid items-center gap-1">
								&nbsp;
								<Button
									className="mb-px"
									type="submit"
									disabled={!idpOrgName || coderOrgs.length === 0}
									onClick={async () => {
										const newSyncSettings = {
											...form.values,
											mapping: {
												...form.values.mapping,
												[idpOrgName]: coderOrgs.map((org) => org.value),
											},
										};
										void form.setFieldValue("mapping", newSyncSettings.mapping);
										form.handleSubmit();
										setIdpOrgName("");
										setCoderOrgs([]);
									}}
								>
									<Plus size={14} />
									Add IdP organization
								</Button>
							</div>
						</div>
						<IdpMappingTable isEmpty={organizationMappingCount === 0}>
							{form.values.mapping &&
								Object.entries(form.values.mapping)
									.sort()
									.map(([idpOrg, organizations]) => (
										<OrganizationRow
											key={idpOrg}
											idpOrg={idpOrg}
											coderOrgs={getOrgNames(organizations)}
											onDelete={handleDelete}
										/>
									))}
						</IdpMappingTable>
					</div>
				</fieldset>
			</form>
		</div>
	);
};

interface IdpMappingTableProps {
	isEmpty: boolean;
	children: React.ReactNode;
}

const IdpMappingTable: FC<IdpMappingTableProps> = ({ isEmpty, children }) => {
	return (
		<TableContainer>
			<Table>
				<TableHead>
					<TableRow>
						<TableCell width="45%">IdP organization</TableCell>
						<TableCell width="55%">Coder organization</TableCell>
						<TableCell width="10%" />
					</TableRow>
				</TableHead>
				<TableBody>
					<ChooseOne>
						<Cond condition={isEmpty}>
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message={"No organization mappings"}
										isCompact
										cta={
											<Button variant="outline" asChild>
												<a
													href={docs("/admin/users/idp-sync")}
													className="no-underline"
												>
													<SquareArrowOutUpRight size={14} />
													How to set up IdP organization sync
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

interface OrganizationRowProps {
	idpOrg: string;
	coderOrgs: readonly string[];
	onDelete: (idpOrg: string) => void;
}

const OrganizationRow: FC<OrganizationRowProps> = ({
	idpOrg,
	coderOrgs,
	onDelete,
}) => {
	return (
		<TableRow data-testid={`idp-org-${idpOrg}`}>
			<TableCell>{idpOrg}</TableCell>
			<TableCell>
				<OrganizationPills organizations={coderOrgs} />
			</TableCell>
			<TableCell>
				<Button
					variant="outline"
					className="w-8 h-8 px-1.5 py-1.5 text-content-secondary"
					aria-label="delete"
					onClick={() => onDelete(idpOrg)}
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
					<Skeleton variant="text" width="10%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

export const AssignDefaultOrgHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger />
			<HelpTooltipContent>
				<HelpTooltipText>
					Disabling will remove all users from the default organization.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

export default IdpOrgSyncPageView;
