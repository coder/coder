import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { getTemplatePageTitle } from "../utils";

const TemplateFilesPage: FC = () => {
	const { organization: organizationName = "default" } = useParams() as {
		organization?: string;
	};
	const { template, activeVersion } = useTemplateLayoutContext();
	const { data: currentFiles } = useQuery(
		templateFiles(activeVersion.job.file_id),
	);
	const previousVersionQuery = useQuery(
		previousTemplateVersion(
			organizationName,
			template.name,
			activeVersion.name,
		),
	);
	const previousVersion = previousVersionQuery.data;
	const hasPreviousVersion =
		previousVersionQuery.isSuccess && previousVersion !== null;
	const { data: previousFiles } = useQuery({
		...templateFiles(previousVersion?.job.file_id ?? ""),
		enabled: hasPreviousVersion,
	});
	const shouldDisplayFiles =
		currentFiles && (!hasPreviousVersion || previousFiles);

	return (
		<>
			<Helmet>
				<title>{getTemplatePageTitle("Source Code", template)}</title>
			</Helmet>

			{shouldDisplayFiles ? (
				<TemplateFiles
					organizationName={template.organization_name}
					templateName={template.name}
					versionName={activeVersion.name}
					currentFiles={currentFiles}
					baseFiles={previousFiles}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default TemplateFilesPage;
