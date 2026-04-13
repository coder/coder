import { useTheme } from "@emotion/react";
import Link from "@mui/material/Link";
import { InfoIcon, RotateCcwIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { useQuery } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { templateVersion } from "#/api/queries/templates";
import type { Workspace } from "#/api/typesGenerated";
import {
	HelpPopover,
	HelpPopoverAction,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
	HelpPopoverTrigger,
} from "#/components/HelpPopover/HelpPopover";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import {
	useWorkspaceUpdate,
	WorkspaceUpdateDialogs,
} from "../WorkspaceUpdateDialogs";

interface WorkspaceOutdatedTooltipProps {
	workspace: Workspace;
	children?: ReactNode;
}

export const WorkspaceOutdatedTooltip: FC<WorkspaceOutdatedTooltipProps> = ({
	workspace,
	children,
}) => {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<HelpPopover open={isOpen} onOpenChange={setIsOpen}>
			{children ? (
				<HelpPopoverTrigger asChild>
					<span className="flex items-center gap-1.5 cursor-help">
						<InfoIcon
							css={(theme) => ({
								color: theme.roles.notice.outline,
							})}
							size={14}
						/>
						<span>{children}</span>
					</span>
				</HelpPopoverTrigger>
			) : (
				<HelpPopoverIconTrigger size="small" hoverEffect={false}>
					<InfoIcon
						css={(theme) => ({
							color: theme.roles.notice.outline,
						})}
					/>
					<span className="sr-only">Outdated info</span>
				</HelpPopoverIconTrigger>
			)}
			<WorkspaceOutdatedTooltipContent isOpen={isOpen} workspace={workspace} />
		</HelpPopover>
	);
};

type TooltipContentProps = WorkspaceOutdatedTooltipProps & { isOpen: boolean };

const WorkspaceOutdatedTooltipContent: FC<TooltipContentProps> = ({
	workspace,
	isOpen,
}) => {
	const getLink = useLinks();
	const theme = useTheme();
	const { data: activeVersion } = useQuery({
		...templateVersion(workspace.template_active_version_id),
		enabled: isOpen,
	});
	const updateWorkspace = useWorkspaceUpdate({
		workspace,
		latestVersion: activeVersion,
		onError: (error) => {
			toast.error(
				getErrorMessage(error, `Error updating workspace "${workspace.name}".`),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	const versionLink = `${getLink(
		linkToTemplate(workspace.organization_name, workspace.template_name),
	)}`;

	return (
		<>
			<HelpPopoverContent disablePortal={false}>
				<HelpPopoverTitle>Outdated</HelpPopoverTitle>
				<HelpPopoverText>
					This workspace version is outdated and a newer version is available.
				</HelpPopoverText>

				<div className="flex flex-col gap-2 py-2 text-[13px]">
					<div className="leading-[1.6]">
						<div className="text-content-primary font-semibold">
							New version
						</div>
						<div>
							{activeVersion ? (
								<Link
									href={`${versionLink}/versions/${activeVersion.name}`}
									target="_blank"
									css={{ color: theme.palette.primary.light }}
								>
									{activeVersion.name}
								</Link>
							) : (
								<Skeleton variant="text" height={20} width={100} />
							)}
						</div>
					</div>

					<div className="leading-[1.6]">
						<div className="text-content-primary font-semibold">Message</div>
						<div>
							{activeVersion ? (
								activeVersion.message || "No message"
							) : (
								<Skeleton variant="text" height={20} width={150} />
							)}
						</div>
					</div>
				</div>

				<HelpPopoverLinksGroup>
					<HelpPopoverAction
						icon={RotateCcwIcon}
						onClick={updateWorkspace.update}
					>
						Update
					</HelpPopoverAction>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
			<WorkspaceUpdateDialogs {...updateWorkspace.dialogs} />
		</>
	);
};
