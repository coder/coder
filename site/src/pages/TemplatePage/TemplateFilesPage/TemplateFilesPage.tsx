import { ExternalLinkIcon } from "lucide-react";
import { useEffect } from "react";
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

const TemplateFilesPage: React.FC = () => {
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
				<Alert
					severity="success"
					dismissible
					className="mb-6"
					actions={
						<>
							<Button asChild size="sm" variant="outline">
								<a
									href="https://registry.coder.com/skills/coder-templates"
									target="_blank"
									rel="noopener noreferrer"
								>
									Open template skill
									<ExternalLinkIcon />
									<span className="sr-only"> (opens in new tab)</span>
								</a>
							</Button>
							<Button asChild size="sm" variant="outline">
								<a
									href={docs("/admin/templates/creating-templates")}
									target="_blank"
									rel="noopener noreferrer"
								>
									View docs
									<ExternalLinkIcon />
									<span className="sr-only"> (opens in new tab)</span>
								</a>
							</Button>
						</>
					}
				>
					<AlertTitle className="font-semibold">Template created</AlertTitle>
					<AlertDescription>
						Edit the Terraform files below to customize{" "}
						{template.display_name || template.name}, or use the template agent
						skill to make changes for you.
					</AlertDescription>
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
