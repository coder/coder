import { PlusIcon } from "lucide-react";
import type { FC } from "react";
import { useLocation, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { MCPServerRow } from "./components/MCPServerRow";

interface MCPServersPageViewProps {
	isLoading: boolean;
	error: unknown;
	servers: readonly TypesGen.MCPServerConfig[];
	canCreateMCPServers: boolean;
	canEditMCPServers: boolean;
}

const MCPServersPageView: FC<MCPServersPageViewProps> = ({
	isLoading,
	error,
	servers,
	canCreateMCPServers,
	canEditMCPServers,
}) => {
	const navigate = useNavigate();
	const location = useLocation();
	const goToAddServer = () =>
		void navigate({
			pathname: "/ai/settings/mcp-servers/add",
			search: location.search,
		});

	return (
		<div>
			<SettingsHeader
				actions={
					canCreateMCPServers ? (
						<Button variant="outline" onClick={goToAddServer}>
							<PlusIcon />
							<span>Add server</span>
						</Button>
					) : undefined
				}
			>
				<SettingsHeaderTitle>MCP servers</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure external MCP servers that provide additional tools for Coder
					Agents.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}
			<Table className="table-fixed min-w-[640px]" aria-label="MCP servers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/2">Name</TableHead>
						<TableHead className="w-1/5">Auth Method</TableHead>
						<TableHead className="w-1/5">Availability</TableHead>
						<TableHead className="w-32">Status</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open server</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">
					{isLoading ? (
						<TableLoader />
					) : !error && servers.length === 0 ? (
						<TableEmpty
							message="No MCP servers configured"
							description="Add a server to give agents access to external tools."
							cta={
								canCreateMCPServers ? (
									<Button variant="outline" onClick={goToAddServer}>
										<PlusIcon />
										<span>Add server</span>
									</Button>
								) : undefined
							}
						/>
					) : (
						servers.map((server) => (
							<MCPServerRow
								key={server.id}
								server={server}
								onClick={() =>
									void navigate({
										pathname: canEditMCPServers
											? `/ai/settings/mcp-servers/${server.id}`
											: `/ai/settings/mcp-servers/${server.id}/sharing`,
										search: location.search,
									})
								}
							/>
						))
					)}
				</TableBody>
			</Table>
		</div>
	);
};

export default MCPServersPageView;
