import {
	templateByName,
	templateFiles,
	templateVersion,
	templateVersionByName,
} from "api/queries/templates";
import { useAuthenticated } from "hooks";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, useMemo } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import TemplateVersionPageView from "./TemplateVersionPageView";

const TemplateVersionPage: FC = () => {
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
	const selectedVersionQuery = useQuery(
		templateVersionByName(organizationName, templateName, versionName),
	);
	const selectedVersionFilesQuery = useQuery({
		...templateFiles(selectedVersionQuery.data?.job.file_id ?? ""),
		enabled: Boolean(selectedVersionQuery.data),
	});
	const activeVersionQuery = useQuery({
		...templateVersion(templateQuery.data?.active_version_id ?? ""),
		enabled: Boolean(templateQuery.data),
	});
	const activeVersionFilesQuery = useQuery({
		...templateFiles(activeVersionQuery.data?.job.file_id ?? ""),
		enabled: Boolean(activeVersionQuery.data),
	});

	const { permissions } = useAuthenticated();
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

	return (
		<>
			<Helmet>
				<title>{pageTitle(versionName, templateName)}</title>
			</Helmet>

			<TemplateVersionPageView
				error={
					templateQuery.error ||
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
					permissions.updateTemplates ? createWorkspaceUrl : undefined
				}
			/>
		</>
	);
};

export default TemplateVersionPage;
