import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import { FC } from "react";
import { Region } from "api/typesGenerated";
import { ProxyRow } from "./WorkspaceProxyRow";
import { ProxyLatencyReport } from "contexts/useProxyLatency";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export interface WorkspaceProxyViewProps {
  proxies?: Region[];
  proxyLatencies?: Record<string, ProxyLatencyReport>;
  getWorkspaceProxiesError?: unknown;
  isLoading: boolean;
  hasLoaded: boolean;
  preferredProxy?: Region;
  selectProxyError?: unknown;
}

export const WorkspaceProxyView: FC<
  React.PropsWithChildren<WorkspaceProxyViewProps>
> = ({
  proxies,
  proxyLatencies,
  getWorkspaceProxiesError,
  isLoading,
  hasLoaded,
  selectProxyError,
}) => {
  return (
    <Stack>
      {Boolean(getWorkspaceProxiesError) && (
        <ErrorAlert error={getWorkspaceProxiesError} />
      )}
      {Boolean(selectProxyError) && <ErrorAlert error={selectProxyError} />}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="40%">Proxy</TableCell>
              <TableCell width="30%">URL</TableCell>
              <TableCell width="10%">Status</TableCell>
              <TableCell width="20%" css={{ textAlign: "right" }}>
                Latency
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={isLoading}>
                <TableLoader />
              </Cond>
              <Cond condition={hasLoaded && proxies?.length === 0}>
                <TableEmpty message="No workspace proxies found" />
              </Cond>
              <Cond>
                {proxies?.map((proxy) => (
                  <ProxyRow
                    latency={proxyLatencies?.[proxy.id]}
                    key={proxy.id}
                    proxy={proxy}
                  />
                ))}
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  );
};
