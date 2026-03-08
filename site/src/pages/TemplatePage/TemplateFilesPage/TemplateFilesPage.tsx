import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Loader } from "components/Loader/Loader";
import { LockIcon } from "lucide-react";
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
	const currentFilesQuery = useQuery(
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

	const hasError = Boolean(currentFilesQuery.error);
	const shouldDisplayFiles =
		currentFilesQuery.data && (!hasPreviousVersion || previousFiles);

	return (
		<>
			<title>{getTemplatePageTitle("Source Code", template)}</title>

			{hasError ? (
				<Alert severity="warning">
					<AlertTitle className="flex items-center gap-2">
						<LockIcon className="size-4" />
						Insufficient permissions to view source code
					</AlertTitle>
					<AlertDescription>
						You do not have permission to view the source code for this
						template. Contact an administrator to request access.
					</AlertDescription>
				</Alert>
			) : shouldDisplayFiles ? (
				<TemplateFiles
					organizationName={template.organization_name}
					templateName={template.name}
					versionName={activeVersion.name}
					currentFiles={currentFilesQuery.data}
					baseFiles={previousFiles}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default TemplateFilesPage;
