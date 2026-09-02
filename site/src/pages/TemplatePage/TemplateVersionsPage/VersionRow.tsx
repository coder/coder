import type { FC } from "react";
import { useNavigate } from "react-router";
import type { TemplateVersion } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { TableCell } from "#/components/Table/Table";
import { TimelineEntry } from "#/components/Timeline/TimelineEntry";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";

interface VersionRowProps {
	version: TemplateVersion;
	isActive: boolean;
	isLatest: boolean;
	onPromoteClick?: (version: TemplateVersion) => void;
	onArchiveClick?: (version: TemplateVersion) => void;
}

export const VersionRow: FC<VersionRowProps> = ({
	version,
	isActive,
	isLatest,
	onPromoteClick,
	onArchiveClick,
}) => {
	const navigate = useNavigate();
	const { permissions } = useAuthenticated();

	const clickableProps = useClickableTableRow({
		onClick: () => navigate(version.name),
	});

	const jobStatus = version.job.status;

	return (
		<TimelineEntry
			data-testid={`version-${version.id}`}
			aria-label={version.name}
			{...(permissions.updateTemplates ? clickableProps : { clickable: false })}
		>
			<TableCell className="relative border-b-0 p-0!">
				<div className="flex flex-row items-center justify-between gap-4 px-8 py-4">
					<div className="flex flex-row items-center gap-4">
						<Avatar
							fallback={version.created_by.username}
							src={version.created_by.avatar_url}
						/>
						<div className="flex flex-row items-center gap-2 font-inherit text-base font-normal leading-normal">
							<span>
								<strong>{version.created_by.username}</strong> created the
								version <strong>{version.name}</strong>
							</span>
							{version.message && (
								<InfoTooltip title="Message" message={version.message} />
							)}
							<span className="text-xs text-content-secondary">
								{new Date(version.created_at).toLocaleTimeString()}
							</span>
						</div>
					</div>
					<div className="flex flex-row items-center gap-4">
						{isActive && (
							<Badge role="status" variant="green">
								Active
							</Badge>
						)}
						{isLatest && <Badge role="status">Newest</Badge>}
						{jobStatus === "pending" && (
							<Badge role="status">Pending&hellip;</Badge>
						)}
						{jobStatus === "running" && (
							<Badge role="status" variant="info">
								Building&hellip;
							</Badge>
						)}
						{(jobStatus === "canceling" || jobStatus === "canceled") && (
							<Badge role="status">Canceled</Badge>
						)}
						{jobStatus === "failed" && (
							<Badge role="status" variant="destructive">
								Failed
							</Badge>
						)}

						{jobStatus === "failed" && onArchiveClick && (
							<Button
								variant="outline"
								disabled={isActive || version.archived}
								onClick={(e) => {
									e.preventDefault();
									e.stopPropagation();
									onArchiveClick?.(version);
								}}
							>
								Archive&hellip;
							</Button>
						)}

						{jobStatus === "succeeded" && onPromoteClick && (
							<Button
								variant="outline"
								disabled={isActive || jobStatus !== "succeeded"}
								onClick={(e) => {
									e.preventDefault();
									e.stopPropagation();
									onPromoteClick?.(version);
								}}
							>
								Promote&hellip;
							</Button>
						)}
					</div>
				</div>
			</TableCell>
		</TimelineEntry>
	);
};
