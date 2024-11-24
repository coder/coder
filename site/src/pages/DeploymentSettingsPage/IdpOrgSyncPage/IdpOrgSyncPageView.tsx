import type { Interpolation, Theme } from "@emotion/react";
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
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { Button } from "components/ui/button";
import { Input } from "components/ui/input";
import { Label } from "components/ui/label";
import MultipleSelector, { type Option } from "components/ui/multiple-selector";
import { Switch } from "components/ui/switch";
import { useFormik } from "formik";
import { Plus, SquareArrowOutUpRight, Trash } from "lucide-react";
import type React from "react";
import { useState } from "react";
import type { FC } from "react";
import { docs } from "utils/docs";
import { OrganizationPills } from "./OrganizationPills";

interface IdpSyncPageViewProps {
	organizationSyncSettings: OrganizationSyncSettings | undefined;
	organizations: readonly Organization[];
	onSubmit: (data: OrganizationSyncSettings) => void;
	error?: unknown;
}

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	organizationSyncSettings,
	organizations,
	onSubmit,
	error,
}) => {
	const form = useFormik<OrganizationSyncSettings>({
		initialValues: {
			field: organizationSyncSettings?.field || "",
			organization_assign_default:
				organizationSyncSettings?.organization_assign_default || true,
			mapping: organizationSyncSettings?.mapping || {},
		},
		// validationSchema,
		onSubmit,
		enableReinitialize: true,
	});
	const [coderOrgs, setCoderOrgs] = useState<Option[]>([]);
	const [idpOrgName, setIdpOrgName] = useState("");
	const [syncSettings, setSyncSettings] = useState(
		organizationSyncSettings || {
			field: "",
			organization_assign_default: true,
			mapping: {},
		},
	);

	const organizationMappingCount = syncSettings?.mapping
		? Object.entries(syncSettings.mapping).length
		: 0;

	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (!organizationSyncSettings) {
		return <Loader />;
	}

	const OPTIONS: Option[] = organizations.map((org) => ({
		label: org.display_name,
		value: org.id,
	}));

	const getOrgNames = (orgIds: readonly string[]) => {
		return orgIds.map(
			(orgId) =>
				organizations.find((org) => org.id === orgId)?.display_name || orgId,
		);
	};

	const handleDelete = async (idpOrg: string) => {
		const newMapping = Object.fromEntries(
			Object.entries(syncSettings?.mapping || {}).filter(
				([key]) => key !== idpOrg,
			),
		);
		const newSyncSettings = {
			...(syncSettings as OrganizationSyncSettings),
			mapping: newMapping,
		};
		setSyncSettings(newSyncSettings);
		await form.setFieldValue("mapping", newSyncSettings.mapping);
	};

	return (
		<>
			<Stack spacing={2}>
				<form onSubmit={form.handleSubmit}>
					<fieldset disabled={form.isSubmitting}>
						<div className="flex flex-row">
							<div className="grid items-center gap-1">
								<Label className="text-sm" htmlFor="sync-field">
									Organization sync field
								</Label>
								<div className="flex flex-row items-center gap-4">
									<Input
										id="sync-field"
										value={syncSettings.field}
										className="w-72"
										onChange={async (
											event: React.ChangeEvent<HTMLInputElement>,
										) => {
											setSyncSettings({
												...(syncSettings as OrganizationSyncSettings),
												field: event.target.value,
											});
											await form.setFieldValue("field", event.target.value);
										}}
									/>
									<div className="flex flex-row items-center gap-3">
										<Switch
											id="organization-assign-default"
											checked={syncSettings.organization_assign_default}
											onCheckedChange={async (checked) => {
												setSyncSettings({
													...(syncSettings as OrganizationSyncSettings),
													organization_assign_default: checked,
												});
												await form.setFieldValue(
													"organization_assign_default",
													checked,
												);
											}}
										/>
										<Label htmlFor="organization-assign-default">
											Assign Default Organization
										</Label>
									</div>
								</div>
								<p className="text-content-secondary text-2xs m-0">
									If empty, organization sync is deactivated
								</p>
							</div>
						</div>

						<div className="flex flex-col gap-4">
							<div className="flex flex-row pt-8 gap-2 justify-between">
								<div className="grid items-center gap-1">
									<Label className="text-sm" htmlFor="idp-organization-name">
										IdP organization name
									</Label>
									<Input
										id="idp-organization-name"
										value={idpOrgName}
										className="min-w-72 w-72"
										onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
											setIdpOrgName(event.target.value);
										}}
									/>
								</div>
								<div className="grid items-center gap-1 flex-1">
									<Label className="text-sm" htmlFor="idp-organization-name">
										Coder organization
									</Label>
									<MultipleSelector
										className="min-w-96 max-w-3xl"
										value={coderOrgs}
										onChange={setCoderOrgs}
										defaultOptions={OPTIONS}
										hidePlaceholderWhenSelected
										placeholder="Select organization"
										emptyIndicator={
											<p className="text-center text-lg leading-10 text-content-primary">
												no results found.
											</p>
										}
									/>
								</div>
								<Button
									className="mb-px self-end"
									disabled={!idpOrgName || coderOrgs.length === 0}
									onClick={async () => {
										const newSyncSettings = {
											...(syncSettings as OrganizationSyncSettings),
											mapping: {
												...syncSettings?.mapping,
												[idpOrgName]: coderOrgs.map((org) => org.value),
											},
										};
										setSyncSettings(newSyncSettings);
										await form.setFieldValue(
											"mapping",
											newSyncSettings.mapping,
										);
										setIdpOrgName("");
										setCoderOrgs([]);
									}}
								>
									<Plus size={14} />
									Add IdP organization
								</Button>
							</div>
							<IdpMappingTable isEmpty={organizationMappingCount === 0}>
								{syncSettings?.mapping &&
									Object.entries(syncSettings.mapping)
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
							<Button
								className="w-20"
								disabled={form.isSubmitting || !form.dirty}
								onClick={(event) => {
									event.preventDefault();
									form.handleSubmit();
								}}
							>
								Save
							</Button>
						</div>
					</fieldset>
				</form>
			</Stack>
		</>
	);
};

interface IdpMappingTableProps {
	isEmpty: boolean;
	children: React.ReactNode;
}

const IdpMappingTable: FC<IdpMappingTableProps> = ({ isEmpty, children }) => {
	const isLoading = false;

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
						<Cond condition={isLoading}>
							<TableLoader />
						</Cond>

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
		<TableRow data-testid={`group-${idpOrg}`}>
			<TableCell>{idpOrg}</TableCell>
			<TableCell>
				<OrganizationPills organizations={coderOrgs} />
			</TableCell>
			<TableCell>
				<Button
					variant="outline"
					className="w-8 h-8 px-1.5 py-1.5 text-content-secondary"
					onClick={() => onDelete(idpOrg)}
				>
					<Trash />
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

export default IdpSyncPageView;
