import type { TemplateVersion } from "api/typesGenerated";
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
import { EditIcon, FileTextIcon, PlusIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { TemplateVersionLogs } from "modules/templates/TemplateVersionLogs/TemplateVersionLogs";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import { createDayString } from "utils/createDayString";
import type { TemplateVersionFiles } from "utils/templateVersion";

export interface TemplateVersionPageViewProps {
	organizationName: string;
	templateName: string;
	versionName: string;
	createWorkspaceUrl?: string;
	error: unknown;
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
}) => {
	const getLink = useLinks();
	const templateLink = getLink(linkToTemplate(organizationName, templateName));
	const [logsOpen, setLogsOpen] = useState(false);

	return (
		<Margins>
			<PageHeader
				actions={
					<>
						{currentVersion && (
							<Button
								variant="outline"
								onClick={() => setLogsOpen(true)}
								aria-label="View build logs"
							>
								<FileTextIcon className="!size-icon-sm" />
								View Logs
							</Button>
						)}
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

			{!currentFiles && !error && <Loader />}

			<Stack spacing={4}>
				{Boolean(error) && <ErrorAlert error={error} />}
				{currentVersion?.message && (
					<TemplateUpdateMessage>
						{currentVersion.message}
					</TemplateUpdateMessage>
				)}
				{currentVersion && currentFiles && (
					<>
						<Stats>
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
						</Stats>

						<TemplateFiles
							organizationName={organizationName}
							templateName={templateName}
							versionName={versionName}
							currentFiles={currentFiles}
							baseFiles={baseFiles}
						/>
					</>
				)}
			</Stack>

			{/* Build Logs Sheet */}
			{currentVersion && (
				<TemplateVersionLogs
					version={currentVersion}
					open={logsOpen}
					onClose={() => setLogsOpen(false)}
				/>
			)}
		</Margins>
	);
};
