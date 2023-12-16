import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import LoadingButton from "@mui/lab/LoadingButton";
import ArrowDownwardOutlined from "@mui/icons-material/ArrowDownwardOutlined";
import { type FC, type ReactNode } from "react";
import type * as TypesGen from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { TableLoader } from "components/TableLoader/TableLoader";
import { Timeline } from "components/Timeline/Timeline";
import { Stack } from "components/Stack/Stack";
import { BuildRow } from "./BuildRow";

export const Language = {
  emptyMessage: "No builds found",
};

export interface BuildsTableProps {
  children?: ReactNode;
  builds: TypesGen.WorkspaceBuild[] | undefined;
  onLoadMoreBuilds: () => void;
  isLoadingMoreBuilds: boolean;
  hasMoreBuilds: boolean;
}

export const BuildsTable: FC<BuildsTableProps> = ({
  builds,
  onLoadMoreBuilds,
  isLoadingMoreBuilds,
  hasMoreBuilds,
}) => {
  return (
    <Stack>
      <TableContainer>
        <Table data-testid="builds-table" aria-describedby="builds table">
          <TableBody>
            {builds ? (
              <Timeline
                items={builds}
                getDate={(build) => new Date(build.created_at)}
                row={(build) => <BuildRow key={build.id} build={build} />}
              />
            ) : (
              <TableLoader />
            )}

            {builds && builds.length === 0 && (
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
      {hasMoreBuilds && (
        <LoadingButton
          onClick={onLoadMoreBuilds}
          loading={isLoadingMoreBuilds}
          loadingPosition="start"
          variant="outlined"
          color="neutral"
          startIcon={<ArrowDownwardOutlined />}
          css={{
            display: "inline-flex",
            margin: "auto",
            borderRadius: "9999px",
          }}
        >
          Load previous builds
        </LoadingButton>
      )}
    </Stack>
  );
};
