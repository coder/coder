import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { Region } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import type { FC } from "react";
import { ProxyRow } from "./WorkspaceProxyRow";

export interface WorkspaceProxyViewProps {
	proxies?: readonly Region[];
	proxyLatencies?: Record<string, ProxyLatencyReport>;
	getWorkspaceProxiesError?: unknown;
	isLoading: boolean;
	hasLoaded: boolean;
	preferredProxy?: Region;
	selectProxyError?: unknown;
}

export const WorkspaceProxyView: FC<WorkspaceProxyViewProps> = ({
	proxies,
	proxyLatencies,
	getWorkspaceProxiesError,
	isLoading,
	hasLoaded,
	selectProxyError,
}) => {
	return (
		<Stack>
			<SettingsHeader
				title="Workspace Proxies"
				description="Workspace proxies improve terminal and web app connections to workspaces."
			/>
			{Boolean(getWorkspaceProxiesError) && (
				<ErrorAlert error={getWorkspaceProxiesError} />
			)}
			{Boolean(selectProxyError) && <ErrorAlert error={selectProxyError} />}
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="70%">Proxy</TableCell>
							<TableCell width="10%" css={{ textAlign: "right" }}>
								Status
							</TableCell>
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
