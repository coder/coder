import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import { AuditLog } from "api/typesGenerated";
import { AuditLogRow } from "./AuditLogRow/AuditLogRow";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import { Timeline } from "components/Timeline/Timeline";
import { AuditHelpTooltip } from "./AuditHelpTooltip";
import { ComponentProps, FC } from "react";
import { AuditPaywall } from "./AuditPaywall";
import { AuditFilter } from "./AuditFilter";

import {
  type PaginationResult,
  PaginationContainer,
} from "components/PaginationWidget/PaginationContainer";

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
};

export interface AuditPageViewProps {
  auditLogs?: AuditLog[];
  isNonInitialPage: boolean;
  isAuditLogVisible: boolean;
  error?: unknown;
  filterProps: ComponentProps<typeof AuditFilter>;
  auditsQuery: PaginationResult;
}

export const AuditPageView: FC<AuditPageViewProps> = ({
  auditLogs,
  isNonInitialPage,
  isAuditLogVisible,
  error,
  filterProps,
  auditsQuery: paginationResult,
}) => {
  const isLoading =
    (auditLogs === undefined || paginationResult.totalRecords === undefined) &&
    !error;

  const isEmpty = !isLoading && auditLogs?.length === 0;

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.title}</span>
            <AuditHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        <PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
      </PageHeader>

      <ChooseOne>
        <Cond condition={isAuditLogVisible}>
          <AuditFilter {...filterProps} />

          <PaginationContainer
            query={paginationResult}
            paginationUnitLabel="logs"
          >
            <TableContainer>
              <Table>
                <TableBody>
                  <ChooseOne>
                    {/* Error condition should just show an empty table. */}
                    <Cond condition={Boolean(error)}>
                      <TableRow>
                        <TableCell colSpan={999}>
                          <EmptyState message="An error occurred while loading audit logs" />
                        </TableCell>
                      </TableRow>
                    </Cond>

                    <Cond condition={isLoading}>
                      <TableLoader />
                    </Cond>

                    <Cond condition={isEmpty}>
                      <ChooseOne>
                        <Cond condition={isNonInitialPage}>
                          <TableRow>
                            <TableCell colSpan={999}>
                              <EmptyState message="No audit logs available on this page" />
                            </TableCell>
                          </TableRow>
                        </Cond>

                        <Cond>
                          <TableRow>
                            <TableCell colSpan={999}>
                              <EmptyState message="No audit logs available" />
                            </TableCell>
                          </TableRow>
                        </Cond>
                      </ChooseOne>
                    </Cond>

                    <Cond>
                      {auditLogs && (
                        <Timeline
                          items={auditLogs}
                          getDate={(log) => new Date(log.time)}
                          row={(log) => (
                            <AuditLogRow key={log.id} auditLog={log} />
                          )}
                        />
                      )}
                    </Cond>
                  </ChooseOne>
                </TableBody>
              </Table>
            </TableContainer>
          </PaginationContainer>
        </Cond>

        <Cond>
          <AuditPaywall />
        </Cond>
      </ChooseOne>
    </Margins>
  );
};
