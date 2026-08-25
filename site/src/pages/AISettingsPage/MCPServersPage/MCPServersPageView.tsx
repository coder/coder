import { PlusIcon, SearchIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { getOrganizationLabel } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
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
import { addMCPServerPath, updateMCPServerPath } from "./organizationParam";

interface MCPServersPageViewProps {
	isLoading: boolean;
	error: unknown;
	servers: readonly TypesGen.MCPServerConfig[];
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization;
	addOrganization?: TypesGen.Organization;
	addOrganizations: readonly TypesGen.Organization[];
	canOpenServer: boolean;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
}

const MCPServersPageView: FC<MCPServersPageViewProps> = ({
	isLoading,
	error,
	servers,
	organizations,
	organization,
	addOrganization,
	addOrganizations,
	canOpenServer,
	onSelectOrganization,
}) => {
	const navigate = useNavigate();
	const [searchQuery, setSearchQuery] = useState("");
	const filteredServers = useMemo(() => {
		const normalizedQuery = searchQuery.trim().toLowerCase();
		if (normalizedQuery.length === 0) {
			return servers;
		}
		return servers.filter((server) =>
			[server.display_name, server.slug, server.url]
				.join(" ")
				.toLowerCase()
				.includes(normalizedQuery),
		);
	}, [servers, searchQuery]);
	// Disambiguate against every organization sharing the page context:
	// other creation targets and the currently selected organization.
	const addButtonLabel =
		addOrganization && addOrganization.id !== organization.id
			? `Add server to ${getOrganizationLabel(addOrganization, [
					...addOrganizations,
					organization,
				])}`
			: undefined;
	const goToAddServer = () => {
		if (addOrganization) {
			void navigate(addMCPServerPath(addOrganization));
		}
	};

	return (
		<div>
			<SettingsHeader
				actions={
					addOrganization && (
						<Button
							variant="outline"
							onClick={goToAddServer}
							aria-label={addButtonLabel}
							title={addButtonLabel}
						>
							<PlusIcon />
							<span>Add server</span>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>MCP servers</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure external MCP servers that provide additional tools for Coder
					Agents.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
				<div className="flex-1">
					<InputGroup>
						<InputGroupAddon>
							<SearchIcon />
						</InputGroupAddon>
						<InputGroupInput
							type="search"
							placeholder="Search servers..."
							aria-label="Search servers"
							value={searchQuery}
							onChange={(event) => setSearchQuery(event.target.value)}
						/>
					</InputGroup>
				</div>
				<OrganizationPicker
					id="mcp-servers-organization"
					className="w-full sm:w-60"
					organizations={organizations}
					organization={organization}
					onChange={onSelectOrganization}
					showLabel={false}
				/>
			</div>
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
								addOrganization ? (
									<Button
										variant="outline"
										onClick={goToAddServer}
										aria-label={addButtonLabel}
										title={addButtonLabel}
									>
										<PlusIcon />
										<span>Add server</span>
									</Button>
								) : undefined
							}
						/>
					) : filteredServers.length === 0 ? (
						<TableEmpty
							message="No servers match your search"
							description="Try a different search term."
						/>
					) : (
						filteredServers.map((server) => (
							<MCPServerRow
								key={server.id}
								server={server}
								onClick={
									canOpenServer
										? () =>
												void navigate(
													updateMCPServerPath(server.id, organization),
												)
										: undefined
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
