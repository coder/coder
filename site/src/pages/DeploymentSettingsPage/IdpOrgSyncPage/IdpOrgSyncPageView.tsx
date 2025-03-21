import type {
	Organization,
	OrganizationSyncSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { Combobox } from "components/Combobox/Combobox";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
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
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useFormik } from "formik";
import { Plus, Trash, TriangleAlert } from "lucide-react";
import { type FC, type KeyboardEventHandler, useId, useState } from "react";
import { docs } from "utils/docs";
import { isUUID } from "utils/uuid";
import * as Yup from "yup";
import { OrganizationPills } from "./OrganizationPills";

interface IdpSyncPageViewProps {
	organizationSyncSettings: OrganizationSyncSettings | undefined;
	claimFieldValues: readonly string[] | undefined;
	organizations: readonly Organization[];
	onSubmit: (data: OrganizationSyncSettings) => void;
	onSyncFieldChange: (value: string) => void;
	error?: unknown;
}

const validationSchema = Yup.object({
	field: Yup.string().trim(),
	organization_assign_default: Yup.boolean(),
	mapping: Yup.object()
		.test(
			"valid-mapping",
			"Invalid organization sync settings mapping structure",
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

export const IdpOrgSyncPageView: FC<IdpSyncPageViewProps> = ({
	organizationSyncSettings,
	claimFieldValues,
	organizations,
	onSubmit,
	onSyncFieldChange,
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
	const [inputValue, setInputValue] = useState("");
	const organizationMappingCount = form.values.mapping
		? Object.entries(form.values.mapping).length
		: 0;
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const id = useId();
	const [open, setOpen] = useState(false);

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

	const handleKeyDown: KeyboardEventHandler<HTMLInputElement> = (event) => {
		if (
			event.key === "Enter" &&
			inputValue &&
			!claimFieldValues?.some((value) => value === inputValue.toLowerCase())
		) {
			event.preventDefault();
			setIdpOrgName(inputValue);
			setInputValue("");
			setOpen(false);
		}
	};

	return (
		<div className="flex flex-col gap-2">
			{Boolean(error) && <ErrorAlert error={error} />}
			<form onSubmit={form.handleSubmit}>
				<fieldset disabled={form.isSubmitting} className="border-none">
					<div className="flex flex-row">
						<div className="grid items-center gap-1">
							<Label className="text-sm" htmlFor={`${id}-sync-field`}>
								Organization sync field
							</Label>
							<div className="flex flex-row items-center gap-5">
								<div className="flex flex-row gap-2 w-72">
									<Input
										id={`${id}-sync-field`}
										value={form.values.field}
										onChange={(event) => {
											void form.setFieldValue("field", event.target.value);
											onSyncFieldChange(event.target.value);
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
										<Spinner loading={form.isSubmitting} />
										Save
									</Button>
								</div>
								<div className="flex flex-row items-center gap-3">
									<Switch
										id={`${id}-assign-default-org`}
										checked={form.values.organization_assign_default}
										onCheckedChange={(checked) => {
											if (!checked) {
												setIsDialogOpen(true);
											} else {
												void form.setFieldValue(
													"organization_assign_default",
													checked,
												);
												form.handleSubmit();
											}
										}}
									/>
									<span className="flex flex-row items-center gap-1">
										<Label htmlFor={`${id}-assign-default-org`}>
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
					{form.errors.field && (
						<p className="text-content-danger text-sm m-0">
							{form.errors.field}
						</p>
					)}
					<div className="flex flex-col gap-7">
						<div className="flex flex-row pt-8 gap-2 justify-between items-start">
							<div className="grid items-center gap-1">
								<Label className="text-sm" htmlFor={`${id}-idp-org-name`}>
									IdP organization name
								</Label>

								{claimFieldValues ? (
									<Combobox
										value={idpOrgName}
										options={claimFieldValues}
										placeholder="Select IdP organization"
										open={open}
										onOpenChange={setOpen}
										inputValue={inputValue}
										onInputChange={setInputValue}
										onKeyDown={handleKeyDown}
										onSelect={(value: string) => {
											setIdpOrgName(value);
											setOpen(false);
										}}
									/>
								) : (
									<Input
										id={`${id}-idp-org-name`}
										value={idpOrgName}
										className="w-72"
										onChange={(event) => {
											setIdpOrgName(event.target.value);
										}}
									/>
								)}
							</div>
							<div className="grid items-center gap-1 flex-1">
								<Label className="text-sm" htmlFor={`${id}-coder-org`}>
									Coder organization
								</Label>
								<MultiSelectCombobox
									inputProps={{
										id: `${id}-coder-org`,
									}}
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
											No organizations found
										</p>
									}
								/>
							</div>
							<div className="grid grid-rows-[28px_auto]">
								<div />
								<Button
									type="submit"
									className="min-w-fit"
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
									<Spinner loading={form.isSubmitting}>
										<Plus size={14} />
									</Spinner>
									Add IdP organization
								</Button>
							</div>
						</div>
						{form.errors.mapping && (
							<p className="text-content-danger text-sm m-0">
								{Object.values(form.errors.mapping || {})}
							</p>
						)}
						<IdpMappingTable isEmpty={organizationMappingCount === 0}>
							{form.values.mapping &&
								Object.entries(form.values.mapping)
									.sort(([a], [b]) =>
										a.toLowerCase().localeCompare(b.toLowerCase()),
									)
									.map(([idpOrg, organizations]) => (
										<OrganizationRow
											key={idpOrg}
											idpOrg={idpOrg}
											coderOrgs={getOrgNames(organizations)}
											onDelete={handleDelete}
											exists={claimFieldValues?.includes(idpOrg)}
										/>
									))}
						</IdpMappingTable>
					</div>
				</fieldset>
			</form>

			<Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
				<DialogContent className="flex flex-col gap-12 max-w-lg">
					<DialogHeader className="flex flex-col gap-4">
						<DialogTitle>
							Switch off default organization assignment
						</DialogTitle>
						<DialogDescription>
							Warning: This will remove all users from the default organization
							unless otherwise specified in an organization mapping defined
							below.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter className="flex flex-row">
						<Button variant="outline" onClick={() => setIsDialogOpen(false)}>
							Cancel
						</Button>
						<Button
							onClick={() => {
								void form.setFieldValue("organization_assign_default", false);
								setIsDialogOpen(false);
								form.handleSubmit();
							}}
							type="submit"
						>
							<Spinner loading={form.isSubmitting} />
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
};

interface IdpMappingTableProps {
	isEmpty: boolean;
	children: React.ReactNode;
}

const IdpMappingTable: FC<IdpMappingTableProps> = ({ isEmpty, children }) => {
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">IdP organization</TableHead>
					<TableHead className="w-3/5">Coder organization</TableHead>
					<TableHead className="w-auto" />
				</TableRow>
			</TableHeader>
			<TableBody>
				<ChooseOne>
					<Cond condition={isEmpty}>
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState
									message={"No organization mappings"}
									isCompact
									cta={
										<Link
											href={docs("/admin/users/idp-sync#organization-sync")}
										>
											How to set up IdP organization sync
										</Link>
									}
								/>
							</TableCell>
						</TableRow>
					</Cond>

					<Cond>{children}</Cond>
				</ChooseOne>
			</TableBody>
		</Table>
	);
};

interface OrganizationRowProps {
	idpOrg: string;
	exists: boolean | undefined;
	coderOrgs: readonly string[];
	onDelete: (idpOrg: string) => void;
}

const OrganizationRow: FC<OrganizationRowProps> = ({
	idpOrg,
	exists = true,
	coderOrgs,
	onDelete,
}) => {
	return (
		<TableRow data-testid={`idp-org-${idpOrg}`}>
			<TableCell>
				<div className="flex flex-row items-center gap-2 text-content-primary">
					{idpOrg}
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
				<OrganizationPills organizations={coderOrgs} />
			</TableCell>
			<TableCell>
				<Button
					variant="outline"
					size="icon"
					className="text-content-primary"
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

export const AssignDefaultOrgHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger />
			<HelpTooltipContent>
				<HelpTooltipText>
					Disabling will remove all users from the default organization if a
					mapping for the default organization is not defined.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

export default IdpOrgSyncPageView;
