import { PlusIcon } from "lucide-react";
import type { FC } from "react";
import { useNavigate } from "react-router";
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
import { OrganizationPicker } from "./components/OrganizationPicker";
import { addMCPServerPath } from "./organizationParam";

interface MCPServersPageViewProps {
	isLoading: boolean;
	error: unknown;
	servers: readonly TypesGen.MCPServerConfig[];
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
}

const MCPServersPageView: FC<MCPServersPageViewProps> = ({
	isLoading,
	error,
	servers,
	organizations,
	organization,
	onSelectOrganization,
}) => {
	const navigate = useNavigate();
	const goToAddServer = () => void navigate(addMCPServerPath(organization));

	return (
		<div>
			<SettingsHeader
				actions={
					<Button variant="outline" onClick={goToAddServer}>
						<PlusIcon />
						<span>Add server</span>
					</Button>
				}
			>
				<SettingsHeaderTitle>MCP servers</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure external MCP servers that provide additional tools for Coder
					Agents.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<OrganizationPicker
				id="mcp-servers-organization"
				className="mb-4"
				organizations={organizations}
				organization={organization}
				onChange={onSelectOrganization}
			/>
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
								<Button variant="outline" onClick={goToAddServer}>
									<PlusIcon />
									<span>Add server</span>
								</Button>
							}
						/>
					) : (
						servers.map((server) => (
							<MCPServerRow
								key={server.id}
								server={server}
								onClick={() =>
									void navigate(`/ai/settings/mcp-servers/${server.id}`)
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
