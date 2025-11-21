import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
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
	const currentFilesQuery = useQuery(templateFiles(activeVersion.job.file_id));
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
	const previousFilesQuery = useQuery({
		...templateFiles(previousVersion?.job.file_id ?? ""),
		enabled: hasPreviousVersion,
	});

	// Handle error case for file access
	if (currentFilesQuery.isError) {
		return (
			<>
				<title>{getTemplatePageTitle("Source Code", template)}</title>
				<ErrorAlert error={currentFilesQuery.error} />
			</>
		);
	}

	// Handle error case for previous version files
	if (previousFilesQuery.isError) {
		return (
			<>
				<title>{getTemplatePageTitle("Source Code", template)}</title>
				<ErrorAlert error={previousFilesQuery.error} />
			</>
		);
	}

	const shouldDisplayFiles =
		currentFilesQuery.data && (!hasPreviousVersion || previousFilesQuery.data);

	return (
		<>
			<title>{getTemplatePageTitle("Source Code", template)}</title>

			{shouldDisplayFiles ? (
				<TemplateFiles
					organizationName={template.organization_name}
					templateName={template.name}
					versionName={activeVersion.name}
					currentFiles={currentFilesQuery.data}
					baseFiles={previousFilesQuery.data}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default TemplateFilesPage;
