import {
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	PlusIcon,
} from "lucide-react";
import { type FC, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatProviderConfig,
	deleteChatProviderConfig,
	updateChatProviderConfig,
} from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { ProviderIcon } from "../AgentsPage/components/ChatModelAdminPanel/ProviderIcon";
import { formatProviderLabel } from "../AgentsPage/utils/modelOptions";

const KNOWN_PROVIDERS = [
	"anthropic",
	"openai",
	"bedrock",
	"azure",
	"google",
	"openai-bridge",
	"openrouter",
	"vercel",
] as const;

const AIProvidersPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const providerConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const createProviderMutation = useMutation(
		createChatProviderConfig(queryClient),
	);
	const updateProviderMutation = useMutation(
		updateChatProviderConfig(queryClient),
	);
	const deleteProviderMutation = useMutation(
		deleteChatProviderConfig(queryClient),
	);

	// Build the list of configured providers from API data.
	const configuredProviders = useMemo(() => {
		const configs = providerConfigsQuery.data ?? [];
		return configs
			.filter((pc) => pc.source === "database" || pc.source === "env_preset")
			.map((pc) => ({
				id: pc.id,
				provider: pc.provider,
				label:
					pc.display_name || formatProviderLabel(pc.provider),
				baseUrl: pc.base_url || "",
				hasApiKey: pc.has_api_key ?? false,
			}));
	}, [providerConfigsQuery.data]);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<div>
				<div className="flex items-start justify-between mb-6">
					<div>
						<h1 className="text-3xl font-semibold mt-0 mb-2">
							Providers
						</h1>
						<p className="text-content-secondary text-sm mt-0 max-w-2xl">
							Connect third-party LLM services like OpenAI, Anthropic, or
							Google. Each provider supplies models that users can select for
							their conversations.
						</p>
					</div>
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button variant="default">
								<PlusIcon />
								Add provider
								<ChevronDownIcon className="size-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="w-[220px]">
							<div className="px-3 py-2 text-xs text-content-secondary">
								Select a provider
							</div>
							{KNOWN_PROVIDERS.map((provider) => (
								<DropdownMenuItem key={provider} className="gap-2.5">
									<ProviderIcon
										provider={provider}
										className="h-6 w-6 shrink-0"
									/>
									{formatProviderLabel(provider)}
								</DropdownMenuItem>
							))}
						</DropdownMenuContent>
					</DropdownMenu>
				</div>

				<Table aria-label="AI providers">
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Base URL</TableHead>
							<TableHead className="w-10" />
							<TableHead className="w-10" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{configuredProviders.length === 0 ? (
							<TableRow>
								<TableCell
									colSpan={4}
									className="text-center text-content-secondary py-8"
								>
									No providers configured
								</TableCell>
							</TableRow>
						) : (
							configuredProviders.map((provider) => (
								<TableRow
									key={provider.id}
									hover
									className="cursor-pointer"
								>
									<TableCell>
										<div className="flex items-center gap-3">
											<ProviderIcon
												provider={provider.provider}
												className="h-8 w-8 shrink-0"
											/>
											<span className="text-sm font-medium text-content-primary">
												{provider.label}
											</span>
										</div>
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-secondary">
											{provider.baseUrl}
										</span>
									</TableCell>
									<TableCell>
										{provider.hasApiKey && (
											<CheckIcon className="size-4 text-content-success" />
										)}
									</TableCell>
									<TableCell>
										<ChevronRightIcon className="size-4 text-content-secondary" />
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>
		</RequirePermission>
	);
};

export default AIProvidersPage;
