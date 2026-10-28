import { useFormik } from "formik";
import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useNavigate, useSearchParams } from "react-router";
import {
	createFoundation,
	foundationPath,
	foundationPermissions,
} from "#/api/queries/foundations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";

import { FoundationDestroyProtection } from "./FoundationDestroyProtection";

const CreateFoundationPage: React.FC = () => {
	const { organizations, experiments } = useDashboard();
	const [params] = useSearchParams();
	const [organizationId, setOrganizationId] = useState(
		() =>
			params.get("organization") ??
			organizations.find((org) => org.is_default)?.id ??
			organizations[0]?.id ??
			"",
	);
	const id = useId();
	const navigate = useNavigate();
	const client = useQueryClient();
	const enabled = experiments.includes("template-resources");
	const organization = organizations.find((org) => org.id === organizationId);
	const permissions = useQuery({
		...foundationPermissions(organizationId),
		enabled: enabled && Boolean(organization),
	});
	const create = useMutation(createFoundation(client, organizationId));
	const canCreate = Boolean(
		organization && permissions.data?.canCreateFoundation && !permissions.error,
	);
	const form = useFormik({
		initialValues: { name: "", prevent_destroy_with_workspaces: true },
		onSubmit: ({ name, prevent_destroy_with_workspaces }) => {
			if (!organization || !canCreate || !name.trim() || create.isPending)
				return;
			create.mutate(
				{
					name: name.trim(),
					provisioner_tags: {},
					prevent_destroy_with_workspaces,
				},
				{
					onSuccess: (foundation) => {
						void navigate(
							`${foundationPath(foundation.organization_id, foundation.id)}?edit=true`,
						);
					},
				},
			);
		},
	});

	return (
		<Margins className="pb-12">
			<title>{pageTitle("Create Foundation")}</title>
			<PageHeader>
				<PageHeaderTitle>Create Foundation</PageHeaderTitle>
				<PageHeaderSubtitle>
					Create shared infrastructure, then edit its Terraform files. No
					template is attached automatically.
				</PageHeaderSubtitle>
			</PageHeader>
			{!enabled ? (
				<ErrorAlert error="Enable the template-resources experiment to manage Foundations." />
			) : (
				<form className="max-w-xl space-y-6" onSubmit={form.handleSubmit}>
					{Boolean(create.error) && <ErrorAlert error={create.error} />}
					<div className="space-y-2">
						<Label htmlFor={`${id}-organization`}>Organization</Label>
						<OrganizationAutocomplete
							id={`${id}-organization`}
							value={organization ?? null}
							options={organizations}
							onChange={(value) => {
								setOrganizationId(value?.id ?? "");
								create.reset();
							}}
							disabled={create.isPending}
							required
						/>
						{!organization && (
							<p className="text-sm text-content-secondary">
								Select an accessible organization to continue.
							</p>
						)}
					</div>
					{permissions.isLoading && <Loader />}
					{Boolean(permissions.error) && (
						<div className="space-y-2">
							<ErrorAlert error={permissions.error} />
							<Button
								type="button"
								variant="outline"
								onClick={() => void permissions.refetch()}
							>
								Retry permissions
							</Button>
						</div>
					)}
					{organization &&
						permissions.data &&
						!permissions.data.canCreateFoundation && (
							<p className="text-sm text-content-secondary">
								Foundation create permission is required in this organization.
							</p>
						)}
					<div className="space-y-2">
						<Label htmlFor={`${id}-name`}>Foundation name</Label>
						<Input
							id={`${id}-name`}
							{...form.getFieldProps("name")}
							disabled={create.isPending}
							required
						/>
					</div>
					<FoundationDestroyProtection
						checked={form.values.prevent_destroy_with_workspaces}
						disabled={create.isPending}
						onChange={(value) =>
							void form.setFieldValue("prevent_destroy_with_workspaces", value)
						}
					/>
					<p className="text-sm text-content-secondary">
						Creating a Foundation does not run Terraform or rebuild workspaces.
					</p>
					<div className="flex justify-end gap-3">
						<Button asChild variant="outline">
							<Link to="/foundations">Cancel</Link>
						</Button>
						<Button
							type="submit"
							disabled={
								!canCreate || !form.values.name.trim() || create.isPending
							}
						>
							{create.isPending ? "Creating…" : "Create Foundation"}
						</Button>
					</div>
				</form>
			)}
		</Margins>
	);
};
export default CreateFoundationPage;
