import { useMemo } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	templateByName,
	templateFiles,
	templateVersion,
	templateVersionByName,
} from "#/api/queries/templates";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { templatePermissionChecks } from "#/modules/permissions/templates";
import { workspacePermissionChecks } from "#/modules/permissions/workspaces";
import { pageTitle } from "#/utils/page";
import { TemplateVersionPageView } from "./TemplateVersionPageView";

const TemplateVersionPage: React.FC = () => {
	const getLink = useLinks();
	const {
		organization: organizationName = "default",
		template: templateName,
		version: versionName,
	} = useParams() as {
		organization?: string;
		template: string;
		version: string;
	};

	/**
	 * Template version files
	 */
	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);
	const { user: me } = useAuthenticated();
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: {
				...templatePermissionChecks(templateQuery.data?.id ?? ""),
				...workspacePermissionChecks(
					templateQuery.data?.organization_id ?? "",
					me.id,
				),
			},
		}),
		enabled: Boolean(templateQuery.data),
	});
	const canUpdateTemplate = permissionsQuery.data?.canUpdateTemplate ?? false;
	const selectedVersionQuery = useQuery(
		templateVersionByName(organizationName, templateName, versionName),
	);
	const selectedVersionFilesQuery = useQuery({
		...templateFiles(selectedVersionQuery.data?.job.file_id ?? ""),
		enabled: canUpdateTemplate && Boolean(selectedVersionQuery.data),
	});
	const activeVersionQuery = useQuery({
		...templateVersion(templateQuery.data?.active_version_id ?? ""),
		enabled: canUpdateTemplate && Boolean(templateQuery.data),
	});
	const activeVersionFilesQuery = useQuery({
		...templateFiles(activeVersionQuery.data?.job.file_id ?? ""),
		enabled: canUpdateTemplate && Boolean(activeVersionQuery.data),
	});

	const versionId = selectedVersionQuery.data?.id;
	const createWorkspaceUrl = useMemo(() => {
		const params = new URLSearchParams();
		if (versionId) {
			params.set("version", versionId);
			return `${getLink(
				linkToTemplate(organizationName, templateName),
			)}/workspace?${params.toString()}`;
		}
		return undefined;
	}, [getLink, templateName, versionId, organizationName]);

	if (
		!permissionsQuery.data &&
		!permissionsQuery.error &&
		!templateQuery.error
	) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle(versionName, templateName)}</title>

			<TemplateVersionPageView
				canUpdateTemplate={canUpdateTemplate}
				error={
					templateQuery.error ||
					permissionsQuery.error ||
					selectedVersionQuery.error ||
					selectedVersionFilesQuery.error ||
					activeVersionQuery.error ||
					activeVersionFilesQuery.error
				}
				currentVersion={selectedVersionQuery.data}
				currentFiles={selectedVersionFilesQuery.data}
				baseFiles={activeVersionFilesQuery.data}
				versionName={versionName}
				templateName={templateName}
				organizationName={organizationName}
				createWorkspaceUrl={
					permissionsQuery.data?.createWorkspaceForUserID
						? createWorkspaceUrl
						: undefined
				}
			/>
		</>
	);
};

export default TemplateVersionPage;
