import type { Region } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import type { FC } from "react";
import { ProxyRow } from "./WorkspaceProxyRow";

interface WorkspaceProxyViewProps {
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
			<SettingsHeader>
				<SettingsHeaderTitle>Workspace Proxies</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Workspace proxies improve terminal and web app connections to
					workspaces.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{Boolean(getWorkspaceProxiesError) && (
				<ErrorAlert error={getWorkspaceProxiesError} />
			)}
			{Boolean(selectProxyError) && <ErrorAlert error={selectProxyError} />}

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[70%]">Proxy</TableHead>
						<TableHead className="w-[10%] text-right">Status</TableHead>
						<TableHead className="w-[20%] text-right">Latency</TableHead>
					</TableRow>
				</TableHeader>
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
		</Stack>
	);
};
