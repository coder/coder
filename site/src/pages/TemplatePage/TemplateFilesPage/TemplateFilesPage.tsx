import { ExternalLinkIcon } from "lucide-react";
import { type FC, useEffect } from "react";
import { useQuery } from "react-query";
import { useLocation, useNavigate, useParams } from "react-router";
import {
	previousTemplateVersion,
	templateFiles,
} from "#/api/queries/templates";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { TemplateFiles } from "#/modules/templates/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { docs } from "#/utils/docs";
import { getTemplatePageTitle } from "../utils";

const TemplateFilesPage: FC = () => {
	const { organization: organizationName = "default" } = useParams() as {
		organization?: string;
	};
	const location = useLocation();
	const navigate = useNavigate();
	const locationState = location.state as { justCreated?: boolean } | null;
	const justCreated = locationState?.justCreated === true;

	useEffect(() => {
		if (justCreated) {
			navigate(location.pathname, { replace: true, state: {} });
		}
	}, [justCreated, navigate, location.pathname]);
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

			{justCreated && (
				<Alert severity="info" dismissible className="mb-6">
					<AlertTitle className="font-semibold">
						Awesome, you just created a new template!
					</AlertTitle>
					<AlertDescription>
						To customize it further you can edit the Terraform or Coder Template
						directly. You can use our template agent skill to help you.
					</AlertDescription>
					<div className="flex items-center gap-2 mt-4">
						<Button asChild size="sm" variant="default">
							<a
								href="https://github.com/coder/registry/blob/main/.agents/skills/coder-templates/SKILL.md"
								target="_blank"
								rel="noopener noreferrer"
								className="flex items-center"
							>
								View agent skill
								<ExternalLinkIcon className="size-icon-sm ml-1" />
							</a>
						</Button>
						<Button asChild size="sm" variant="outline">
							<a
								href={docs("/admin/templates/creating-templates")}
								target="_blank"
								rel="noopener noreferrer"
								className="flex items-center"
							>
								View docs
								<ExternalLinkIcon className="size-icon-sm ml-1" />
							</a>
						</Button>
					</div>
				</Alert>
			)}

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
