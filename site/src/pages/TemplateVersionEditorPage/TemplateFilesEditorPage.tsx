import { ChevronLeftIcon } from "lucide-react";
import { useQuery } from "react-query";
import { Link, Navigate, useParams, useSearchParams } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import { templateByName } from "#/api/queries/templates";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Topbar,
	TopbarData,
	TopbarIconButton,
} from "#/components/FullPageLayout/Topbar";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { ProvisioningScopeNavigation } from "./ProvisioningScopeNavigation";
import { SharedResourceFilesEditor } from "./SharedResourceFilesEditor";
import TemplateVersionEditorPage from "./TemplateVersionEditorPage";

const TemplateFilesEditorPage: React.FC = () => {
	const { experiments } = useDashboard();
	const [params] = useSearchParams();
	const enabled = experiments.includes("template-resources");
	if (params.get("scope") === "template") {
		if (!enabled) {
			const perWorkspaceParams = new URLSearchParams(params);
			perWorkspaceParams.delete("scope");
			return (
				<Navigate to={{ search: perWorkspaceParams.toString() }} replace />
			);
		}
		return <SharedResourcesEditorPage />;
	}
	return (
		<TemplateVersionEditorPage
			scopeNavigation={
				enabled && <ProvisioningScopeNavigation scope="per-workspace" />
			}
		/>
	);
};

const SharedResourcesEditorPage: React.FC = () => {
	const { organization = "default", template: templateName = "" } = useParams();
	const getLink = useLinks();
	const templateQuery = useQuery(templateByName(organization, templateName));
	const template = templateQuery.data;
	const permissions = useQuery({
		...checkAuthorization({
			checks: {
				canUpdateTemplate: {
					object: {
						resource_type: "template",
						resource_id: template?.id ?? "",
					},
					action: "update",
				},
			},
		}),
		enabled: Boolean(template),
	});
	return (
		<div className="h-full flex flex-col">
			<title>{pageTitle(templateName, "Shared resources editor")}</title>
			{!(template && permissions.data?.canUpdateTemplate) && (
				<>
					<Topbar className="flex items-center gap-4 pr-4">
						<TopbarIconButton asChild>
							<Link
								to={getLink(linkToTemplate(organization, templateName))}
								aria-label="Back to the template"
							>
								<ChevronLeftIcon />
							</Link>
						</TopbarIconButton>
						<TopbarData>
							{template?.display_name || template?.name || templateName}
						</TopbarData>
					</Topbar>
					<ProvisioningScopeNavigation scope="shared" />
				</>
			)}
			{Boolean(templateQuery.error || permissions.error) && (
				<div className="space-y-4">
					<ErrorAlert error={templateQuery.error || permissions.error} />
					<Button
						variant="outline"
						onClick={() => {
							void templateQuery.refetch();
							if (template) void permissions.refetch();
						}}
					>
						Retry editor
					</Button>
				</div>
			)}
			{(templateQuery.isLoading || permissions.isLoading) && <Loader />}
			{template &&
				permissions.data &&
				(permissions.data.canUpdateTemplate ? (
					<SharedResourceFilesEditor template={template} />
				) : (
					<ErrorAlert error="Template update permission is required to view shared resources." />
				))}
		</div>
	);
};

export default TemplateFilesEditorPage;
