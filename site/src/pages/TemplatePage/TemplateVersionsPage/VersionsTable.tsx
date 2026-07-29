import type { FC } from "react";
import type { TemplateVersion } from "#/api/typesGenerated";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { Timeline } from "#/components/Timeline/Timeline";
import { VersionRow } from "./VersionRow";

interface VersionsTableProps {
	activeVersionId: string;
	versions?: TemplateVersion[];
	onPromoteClick?: (version: TemplateVersion) => void;
	onArchiveClick?: (version: TemplateVersion) => void;
}

export const VersionsTable: FC<VersionsTableProps> = ({
	activeVersionId,
	versions,
	onArchiveClick,
	onPromoteClick,
}) => {
	const latestVersionId = versions?.reduce<TemplateVersion | undefined>(
		(latestSoFar, against) => {
			if (against.job.status !== "succeeded") {
				return latestSoFar;
			}

			if (!latestSoFar) {
				return against;
			}

			return new Date(against.updated_at).getTime() >
				new Date(latestSoFar.updated_at).getTime()
				? against
				: latestSoFar;
		},
		undefined,
	)?.id;

	return (
		<Table data-testid="versions-table">
			<TableBody>
				{versions ? (
					<Timeline
						items={[...versions].reverse()}
						getDate={(version) => new Date(version.created_at)}
						row={(version) => (
							<VersionRow
								onArchiveClick={onArchiveClick}
								onPromoteClick={onPromoteClick}
								version={version}
								key={version.id}
								isActive={activeVersionId === version.id}
								isLatest={latestVersionId === version.id}
							/>
						)}
					/>
				) : (
					<TableLoader />
				)}

				{versions && versions.length === 0 && (
					<TableRow>
						<TableCell colSpan={999}>
							<div className="p-8">
								<EmptyState message="No versions found" />
							</div>
						</TableCell>
					</TableRow>
				)}
			</TableBody>
		</Table>
	);
};
