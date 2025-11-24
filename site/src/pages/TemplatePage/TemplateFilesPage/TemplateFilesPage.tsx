import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
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
			<title>{getTemplatePageTitle("Source Code", template)}</title>

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
