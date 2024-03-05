import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { TableLoader } from "components/TableLoader/TableLoader";
import { Timeline } from "components/Timeline/Timeline";
import { VersionRow } from "./VersionRow";

export const Language = {
  emptyMessage: "No versions found",
  nameLabel: "Version name",
  createdAtLabel: "Created at",
  createdByLabel: "Created by",
};

export interface VersionsTableProps {
  activeVersionId: string;
  versions?: TypesGen.TemplateVersion[];
  onPromoteClick?: (templateVersionId: string) => void;
  onArchiveClick?: (templateVersionId: string) => void;
}

export const VersionsTable: FC<VersionsTableProps> = ({
  activeVersionId,
  versions,
  onArchiveClick,
  onPromoteClick,
}) => {
  const latestVersionId = versions?.reduce(
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
    undefined as TypesGen.TemplateVersion | undefined,
  )?.id;

  return (
    <TableContainer>
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
                <div css={{ padding: 32 }}>
                  <EmptyState message={Language.emptyMessage} />
                </div>
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
