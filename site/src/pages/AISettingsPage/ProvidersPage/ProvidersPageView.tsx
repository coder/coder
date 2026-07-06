import { ChevronDownIcon, PlusIcon } from "lucide-react";
import { useNavigate } from "react-router";
import type { AIProvider } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Link } from "#/components/Link/Link";
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
import { addableProviders } from "#/pages/AISettingsPage/ProvidersPage/components/addableProviderTypes";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { ProviderRow } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderRow";
import { docs } from "#/utils/docs";

interface ProvidersPageViewProps {
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	providers: AIProvider[];
}

const AddProviderDropdown: React.FC<{ align?: "start" | "end" }> = ({
	align = "end",
}) => {
	const navigate = useNavigate();
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline">
					<PlusIcon />
					<span>Add provider</span>
					<ChevronDownIcon className="ml-1 size-icon-xs" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align={align} className="min-w-56">
				<div className="px-2 py-1.5 text-xs font-medium text-content-secondary">
					Select a provider
				</div>
				{addableProviders.map((entry) => (
					<DropdownMenuItem
						key={entry.value}
						onSelect={() =>
							void navigate(
								`/ai/settings/providers/add?type=${encodeURIComponent(entry.value)}`,
							)
						}
					>
						<ProviderIcon provider={entry.value} />
						<span>{entry.label}</span>
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const ProvidersPageView: React.FC<ProvidersPageViewProps> = ({
	isLoading,
	isFetching,
	error,
	providers,
}) => {
	const navigate = useNavigate();

	return (
		<div>
			<SettingsHeader actions={<AddProviderDropdown />}>
				<SettingsHeaderTitle>Providers</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Connect third-party services like OpenAI, Anthropic, or Amazon
					Bedrock. Providers configured here power Coder Agents, AI Gateway, and
					other capabilities such as APIs, CLI or IDEs that use LLMs. By
					default, users can supply their own keys for any provider.{" "}
					<Link href={docs("/ai-coder/ai-gateway/setup#configure-providers")}>
						View docs
					</Link>
				</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}
			<Table className="table-fixed" aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/3">Base URL</TableHead>
						<TableHead className="w-22">Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">
					{isLoading || isFetching ? (
						<TableLoader />
					) : providers.length === 0 ? (
						<TableEmpty
							message="No providers configured"
							cta={<AddProviderDropdown align="start" />}
						/>
					) : (
						providers.map((provider) => (
							<ProviderRow
								key={provider.name}
								provider={provider}
								onClick={() =>
									navigate(`/ai/settings/providers/${provider.name}`)
								}
							/>
						))
					)}
				</TableBody>
			</Table>
		</div>
	);
};

export default ProvidersPageView;
