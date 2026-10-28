import { useQuery } from "react-query";
import { Link } from "react-router";
import {
	foundationPath,
	foundationPermissions,
	templateFoundation,
} from "#/api/queries/foundations";
import type { Template } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { FoundationBinding } from "./FoundationBinding";

const TemplateSharedResourcesPage: React.FC = () => {
	const { template, permissions } = useTemplateLayoutContext();
	return (
		<SharedResourceManagement
			template={template}
			canUpdateTemplate={permissions.canUpdateTemplate}
		/>
	);
};

type SharedResourceManagementProps = {
	template: Template;
	canUpdateTemplate: boolean;
};

/** Loads only the Foundation association; no workspace version is required. */
export const SharedResourceManagement: React.FC<
	SharedResourceManagementProps
> = ({ template, canUpdateTemplate }) => {
	const { experiments } = useDashboard();
	const enabled = experiments.includes("template-resources");
	const permissions = useQuery({
		...foundationPermissions(template.organization_id),
		enabled,
	});
	const query = useQuery({
		...templateFoundation(template.id),
		enabled: enabled && Boolean(permissions.data?.canReadFoundation),
	});
	if (!enabled)
		return (
			<ErrorAlert error="Enable the template-resources experiment to manage Foundations." />
		);
	return (
		<>
			{Boolean(query.error || permissions.error) && (
				<div className="space-y-3 p-4">
					<ErrorAlert error={query.error || permissions.error} />
					<Button
						variant="outline"
						onClick={() => {
							void permissions.refetch();
							if (permissions.data?.canReadFoundation) void query.refetch();
						}}
					>
						Retry Foundation
					</Button>
				</div>
			)}
			{(query.isLoading || permissions.isLoading) && <Loader />}
			{permissions.data && !permissions.data.canReadFoundation && (
				<ErrorAlert error="Foundation read permission is required to view this association." />
			)}
			{query.data === null && (
				<FoundationBinding
					template={template}
					canCreate={Boolean(permissions.data?.canCreateFoundation)}
					canBind={canUpdateTemplate}
				/>
			)}
			{query.data && (
				<section className="space-y-4 p-6" aria-label="Template Foundation">
					<h2 className="text-xl font-semibold">Foundation</h2>
					<p className="text-content-secondary">
						This template uses {query.data.name}. Manage its files, runs, and
						settings on the Foundation page.
					</p>
					<Button asChild variant="outline">
						<Link
							to={foundationPath(query.data.organization_id, query.data.id)}
						>
							{query.data.name}
						</Link>
					</Button>
				</section>
			)}
		</>
	);
};

export default TemplateSharedResourcesPage;
