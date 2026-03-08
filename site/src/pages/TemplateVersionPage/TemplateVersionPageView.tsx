import type { TemplateVersion } from "api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderCaption,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Stats, StatsItem } from "components/Stats/Stats";
import { EditIcon, ExternalLinkIcon, LockIcon, PlusIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { createDayString } from "utils/createDayString";
import type { TemplateVersionFiles } from "utils/templateVersion";

export interface TemplateVersionPageViewProps {
	organizationName: string;
	templateName: string;
	versionName: string;
	createWorkspaceUrl?: string;
	error: unknown;
	filesError: unknown;
	currentVersion: TemplateVersion | undefined;
	currentFiles: TemplateVersionFiles | undefined;
	baseFiles: TemplateVersionFiles | undefined;
}

export const TemplateVersionPageView: FC<TemplateVersionPageViewProps> = ({
	organizationName,
	templateName,
	versionName,
	createWorkspaceUrl,
	currentVersion,
	currentFiles,
	baseFiles,
	error,
	filesError,
}) => {
	const getLink = useLinks();
	const templateLink = getLink(linkToTemplate(organizationName, templateName));
	const hasFilesPermissionError = Boolean(filesError);

	return (
		<Margins>
			<PageHeader
				actions={
					<>
						{createWorkspaceUrl && (
							<Button asChild>
								<RouterLink to={createWorkspaceUrl}>
									<PlusIcon />
									Create workspace
								</RouterLink>
							</Button>
						)}
						<Button variant="outline" asChild>
							<RouterLink to={`${templateLink}/versions/${versionName}/edit`}>
								<EditIcon className="!size-icon-sm" />
								Edit
							</RouterLink>
						</Button>
					</>
				}
			>
				<PageHeaderCaption>Version</PageHeaderCaption>
				<PageHeaderTitle>{versionName}</PageHeaderTitle>
			</PageHeader>

			{!currentVersion && !error && <Loader />}

			<Stack spacing={4}>
				{Boolean(error) && <ErrorAlert error={error} />}
				{currentVersion?.message && (
					<TemplateUpdateMessage>
						{currentVersion.message}
					</TemplateUpdateMessage>
				)}
				{currentVersion && (
					<>
						<Stats className="justify-between">
							<div className="flex flex-wrap items-center">
								<StatsItem
									label="Template"
									value={
										<RouterLink to={templateLink}>{templateName}</RouterLink>
									}
								/>
								<StatsItem
									label="Created by"
									value={currentVersion.created_by.username}
								/>
								<StatsItem
									label="Created"
									value={createDayString(currentVersion.created_at)}
								/>
							</div>
							<a
								href={`/api/v2/templateversions/${currentVersion.id}/logs?format=text`}
								target="_blank"
								rel="noopener noreferrer"
								className="flex items-center gap-1 p-2 text-xs text-content-secondary underline hover:text-content-primary md:py-3.5 md:px-4"
							>
								View raw logs
								<ExternalLinkIcon className="size-3" />
							</a>
						</Stats>

						{hasFilesPermissionError ? (
							<Alert severity="warning">
								<AlertTitle className="flex items-center gap-2">
									<LockIcon className="size-4" />
									Insufficient permissions to view source code
								</AlertTitle>
								<AlertDescription>
									You do not have permission to view the source code for this
									template version. Contact an administrator to request access.
								</AlertDescription>
							</Alert>
						) : currentFiles ? (
							<TemplateFiles
								organizationName={organizationName}
								templateName={templateName}
								versionName={versionName}
								currentFiles={currentFiles}
								baseFiles={baseFiles}
							/>
						) : (
							<Loader />
						)}
					</>
				)}
			</Stack>
		</Margins>
	);
};
