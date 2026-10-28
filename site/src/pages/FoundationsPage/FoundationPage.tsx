import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router";
import {
	createFoundationRun,
	foundation,
	foundationPath,
	foundationPermissions,
} from "#/api/queries/foundations";
import type { Foundation } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { LinkTabs, LinkTabsList, TabLink } from "#/components/Tabs/Tabs";
import { Navbar } from "#/modules/dashboard/Navbar/Navbar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { SharedResourceRunHistory } from "#/pages/TemplatePage/TemplateSharedResourcesPage/SharedResourceRunHistory";
import { SharedResourceFilesEditor } from "#/pages/TemplateVersionEditorPage/SharedResourceFilesEditor";
import { FoundationDelete } from "./FoundationDelete";
import { FoundationImpact } from "./FoundationImpact";
import { FoundationPageHeader } from "./FoundationPageHeader";
import { FoundationResources } from "./FoundationResources";
import { FoundationSettings } from "./FoundationSettings";
import { FoundationSource } from "./FoundationSource";
import { FoundationVersions } from "./FoundationVersions";

type FoundationContentProps = {
	resources: Foundation;
	edit?: boolean;
};

/** Common management surface for attached and standalone Foundations. */
export const FoundationContent: React.FC<FoundationContentProps> = ({
	resources,
	edit = false,
}) => {
	const client = useQueryClient();
	const [params] = useSearchParams();
	const requestedTab = params.get("tab") ?? "files";
	const tab = ["files", "resources", "versions", "runs", "settings"].includes(
		requestedTab,
	)
		? requestedTab
		: "files";
	const path = foundationPath(resources.organization_id, resources.id);
	const permissions = useQuery(
		foundationPermissions(resources.organization_id, resources.id),
	);
	const run = useMutation(
		createFoundationRun(client, resources.organization_id, resources.id),
	);
	if (permissions.isLoading) return <Loader />;
	if (!permissions.data)
		return (
			<Margins>
				<ErrorAlert error={permissions.error} />
				<Button onClick={() => void permissions.refetch()}>
					Retry permissions
				</Button>
			</Margins>
		);
	if (!permissions.data.canReadFoundation)
		return <ErrorAlert error="Foundation read permission is required." />;
	const canUpdate = Boolean(permissions.data.canUpdateFoundation);
	const canUse = Boolean(permissions.data.canUseFoundation);
	return (
		<>
			{Boolean(permissions.error) && (
				<Margins>
					<ErrorAlert error={permissions.error} />
					<Button onClick={() => void permissions.refetch()}>
						Retry permissions
					</Button>
				</Margins>
			)}
			{edit ? (
				<SharedResourceFilesEditor
					key={resources.id}
					resources={resources}
					canUpdate={canUpdate}
					canUse={canUse}
				/>
			) : (
				<div className="pb-12">
					<FoundationPageHeader resources={resources} canUpdate={canUpdate} />
					<LinkTabs active={tab} className="mb-10 -mt-3">
						<Margins>
							<LinkTabsList>
								<TabLink to={`${path}?tab=files`} value="files">
									Source Code
								</TabLink>
								<TabLink to={`${path}?tab=resources`} value="resources">
									Resources
								</TabLink>
								<TabLink to={`${path}?tab=versions`} value="versions">
									Versions
								</TabLink>
								<TabLink to={`${path}?tab=runs`} value="runs">
									Runs
								</TabLink>
								{tab === "settings" && (
									<TabLink to={`${path}?tab=settings`} value="settings">
										Settings
									</TabLink>
								)}
							</LinkTabsList>
						</Margins>
					</LinkTabs>
					<Margins>
						{resources.status === "unknown" && (
							<div className="mb-6">
								<ErrorAlert error="The Foundation state is unknown. Review the latest job logs. Manual reconciliation is required before Terraform can run again." />
							</div>
						)}
						{tab === "files" && (
							<FoundationSource resources={resources} canUpdate={canUpdate} />
						)}
						{tab === "resources" && (
							<FoundationResources resources={resources} />
						)}
						{tab === "versions" && (
							<FoundationVersions
								resources={resources}
								canUpdate={canUpdate}
								canUse={canUse}
								editFilesHref={`${path}?edit=true`}
								isRunning={run.isPending}
								runError={run.error}
								onRun={(request, onSuccess) =>
									run.mutate(request, { onSuccess })
								}
							/>
						)}
						{tab === "runs" && (
							<SharedResourceRunHistory resources={resources} />
						)}
						{tab === "settings" && (
							<div className="space-y-10">
								<div className="max-w-screen-md">
									<FoundationSettings
										key={resources.id}
										resources={resources}
										canUpdate={canUpdate}
									/>
								</div>
								<FoundationImpact resources={resources} />
								<FoundationDelete
									resources={resources}
									canDelete={Boolean(permissions.data.canDeleteFoundation)}
								/>
							</div>
						)}
					</Margins>
				</div>
			)}
		</>
	);
};

const FoundationPage: React.FC = () => {
	const { organization = "", foundation: id = "" } = useParams();
	const [params] = useSearchParams();
	const { experiments, organizations } = useDashboard();
	const enabled = experiments.includes("template-resources");
	const organizationId =
		organizations.find(
			(org) => org.id === organization || org.name === organization,
		)?.id ?? organization;
	const query = useQuery({ ...foundation(organizationId, id), enabled });
	if (!enabled)
		return (
			<ErrorAlert error="Enable the template-resources experiment to manage Foundations." />
		);
	const editing = params.get("edit") === "true";
	return (
		<div className={editing ? "h-full flex flex-col" : undefined}>
			<title>{query.data?.name ?? "Foundation"} - Coder</title>
			{!editing && <Navbar />}
			{Boolean(query.error) && (
				<Margins>
					<ErrorAlert error={query.error} />
					<Button onClick={() => void query.refetch()}>Retry Foundation</Button>
				</Margins>
			)}
			{query.isLoading && <Loader />}
			{query.data && (
				<FoundationContent
					key={query.data.id}
					resources={query.data}
					edit={editing}
				/>
			)}
		</div>
	);
};
export default FoundationPage;
