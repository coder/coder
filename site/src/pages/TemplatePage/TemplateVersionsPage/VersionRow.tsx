import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { useNavigate } from "react-router";
import type { TemplateVersion } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Pill } from "#/components/Pill/Pill";
import { TableCell } from "#/components/Table/Table";
import { TimelineEntry } from "#/components/Timeline/TimelineEntry";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";

interface VersionRowProps {
	version: TemplateVersion;
	isActive: boolean;
	isLatest: boolean;
	onPromoteClick?: (templateVersionId: string) => void;
	onArchiveClick?: (templateVersionId: string) => void;
}

export const VersionRow: FC<VersionRowProps> = ({
	version,
	isActive,
	isLatest,
	onPromoteClick,
	onArchiveClick,
}) => {
	const navigate = useNavigate();

	const clickableProps = useClickableTableRow({
		onClick: () => navigate(version.name),
	});

	const jobStatus = version.job.status;

	return (
		<TimelineEntry
			data-testid={`version-${version.id}`}
			{...clickableProps}
			className={clickableProps.className}
		>
			<TableCell css={styles.versionCell}>
				<div
					className="flex flex-row items-center justify-between gap-4"
					css={styles.versionWrapper}
				>
					<div className="flex flex-row items-center gap-4">
						<Avatar
							fallback={version.created_by.username}
							src={version.created_by.avatar_url}
						/>
						<div
							className="flex flex-row items-center gap-2"
							css={styles.versionSummary}
						>
							<span>
								<strong>{version.created_by.username}</strong> created the
								version <strong>{version.name}</strong>
							</span>
							{version.message && (
								<InfoTooltip title="Message" message={version.message} />
							)}
							<span css={styles.versionTime}>
								{new Date(version.created_at).toLocaleTimeString()}
							</span>
						</div>
					</div>
					<div className="flex flex-row items-center gap-4">
						{isActive && (
							<Pill role="status" type="success">
								Active
							</Pill>
						)}
						{isLatest && (
							<Pill role="status" type="info">
								Newest
							</Pill>
						)}
						{jobStatus === "pending" && (
							<Pill role="status" type="inactive">
								Pending&hellip;
							</Pill>
						)}
						{jobStatus === "running" && (
							<Pill role="status" type="active">
								Building&hellip;
							</Pill>
						)}
						{(jobStatus === "canceling" || jobStatus === "canceled") && (
							<Pill role="status" type="inactive">
								Canceled
							</Pill>
						)}
						{jobStatus === "failed" && (
							<Pill role="status" type="error">
								Failed
							</Pill>
						)}

						{jobStatus === "failed" && onArchiveClick && (
							<Button
								variant="outline"
								disabled={isActive || version.archived}
								onClick={(e) => {
									e.preventDefault();
									e.stopPropagation();
									onArchiveClick?.(version.id);
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
									onPromoteClick?.(version.id);
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

const styles = {
	versionWrapper: {
		padding: "16px 32px",
	},

	versionCell: {
		padding: "0 !important",
		position: "relative",
		borderBottom: 0,
	},

	versionSummary: (theme) => ({
		...(theme.typography.body1 as CSSObject),
		fontFamily: "inherit",
	}),

	versionTime: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 12,
	}),
} satisfies Record<string, Interpolation<Theme>>;
