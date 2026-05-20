import { ChevronDownIcon, PlusIcon } from "lucide-react";
import { useNavigate } from "react-router";
import type { AIProvider } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
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

interface ProvidersPageViewProps {
	isLoading: boolean;
	isFetching: boolean;
	providers: AIProvider[];
}

const ProvidersPageView: React.FC<ProvidersPageViewProps> = ({
	isLoading,
	isFetching,
	providers,
}) => {
	const navigate = useNavigate();

	return (
		<div className="px-8">
			<PageHeader
				className="pt-4 pb-8"
				actions={
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button variant="outline">
								<PlusIcon />
								<span>Add provider</span>
								<ChevronDownIcon className="ml-1 size-icon-xs" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="min-w-56">
							<div className="px-2 py-1.5 text-xs font-medium text-content-secondary">
								Select a provider
							</div>
							{addableProviders.map((entry) => (
								<DropdownMenuItem
									key={entry.value}
									onSelect={() =>
										void navigate(
											`/ai/settings/add?type=${encodeURIComponent(entry.value)}`,
										)
									}
								>
									<ProviderIcon provider={entry.value} />
									<span>{entry.label}</span>
								</DropdownMenuItem>
							))}
						</DropdownMenuContent>
					</DropdownMenu>
				}
			>
				<PageHeaderTitle>Providers</PageHeaderTitle>
				<PageHeaderSubtitle className="max-w-2xl w-full">
					Connect third-party LLM services like OpenAI, Anthropic, or Amazon
					Bedrock. Each provider supplies models that users can select for their
					conversations.
				</PageHeaderSubtitle>
			</PageHeader>
			<Table className="table-fixed" aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/3">Base URL</TableHead>
						<TableHead className="w-22 text-center">Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading || isFetching ? (
						<TableLoader />
					) : providers.length === 0 ? (
						<TableEmpty message="No providers available" />
					) : (
						providers.map((provider) => (
							<ProviderRow
								key={provider.name}
								provider={provider}
								onClick={() => navigate(`/ai/settings/${provider.name}`)}
							/>
						))
					)}
				</TableBody>
			</Table>
		</div>
	);
};

export default ProvidersPageView;
