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
import type { MCPServerFormVariant } from "./components/mcpServerFormLogic";

interface MCPServersPageViewProps {
	isLoading: boolean;
	error: unknown;
	servers: readonly TypesGen.MCPServerConfig[];
	/** Route prefix for the add and edit sub-pages. */
	basePath?: string;
	description?: string;
	/** "personal" hides the deployment availability column. */
	variant?: MCPServerFormVariant;
}

const MCPServersPageView: FC<MCPServersPageViewProps> = ({
	isLoading,
	error,
	servers,
	basePath = "/ai/settings/mcp-servers",
	description = "Configure external MCP servers that provide additional tools for Coder Agents.",
	variant = "deployment",
}) => {
	const navigate = useNavigate();
	const goToAddServer = () => void navigate(`${basePath}/add`);
	const showAvailability = variant === "deployment";
	const title = variant === "personal" ? "Personal MCP servers" : "MCP servers";

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
				<SettingsHeaderTitle>{title}</SettingsHeaderTitle>
				<SettingsHeaderDescription>{description}</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}
			<Table className="table-fixed" aria-label="MCP servers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/2">Name</TableHead>
						<TableHead className="w-1/5">Auth Method</TableHead>
						{showAvailability && (
							<TableHead className="w-1/5">Availability</TableHead>
						)}
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
								showAvailability={showAvailability}
								onClick={() => void navigate(`${basePath}/${server.id}`)}
							/>
						))
					)}
				</TableBody>
			</Table>
		</div>
	);
};

export default MCPServersPageView;
