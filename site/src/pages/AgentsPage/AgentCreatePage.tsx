import { chatModelConfigs, chatModels, createChat } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PanelLeftIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { NavLink, useNavigate, useOutletContext } from "react-router";
import { AgentCreateForm, type CreateChatOptions } from "./AgentCreateForm";
import type { AgentsOutletContext } from "./AgentsPageView";
import { ChimeButton } from "./ChimeButton";
import {
	getModelOptionsFromCatalog,
	getNormalizedModelRef,
} from "./modelOptions";
import { WebPushButton } from "./WebPushButton";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const nilUUID = "00000000-0000-0000-0000-000000000000";
const EMPTY_MODEL_CONFIGS: TypesGen.ChatModelConfig[] = [];

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { appearance } = useDashboard();
	const logoUrl = appearance.logo_url;

	const { isSidebarCollapsed, onExpandSidebar } =
		useOutletContext<AgentsOutletContext>();

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));

	const catalogModelOptions = getModelOptionsFromCatalog(
		chatModelsQuery.data,
		chatModelConfigsQuery.data,
	);

	const modelConfigIDByModelID = (() => {
		const byModelID = new Map<string, string>();
		for (const config of chatModelConfigsQuery.data ?? []) {
			const { provider, model } = getNormalizedModelRef(config);
			if (!provider || !model) {
				continue;
			}
			const colonRef = `${provider}:${model}`;
			if (!byModelID.has(colonRef)) {
				byModelID.set(colonRef, config.id);
			}
			const slashRef = `${provider}/${model}`;
			if (!byModelID.has(slashRef)) {
				byModelID.set(slashRef, config.id);
			}
		}
		return byModelID;
	})();

	const handleCreateChat = async (options: CreateChatOptions) => {
		const { message, fileIDs, workspaceId, model } = options;
		const modelConfigID =
			(model && modelConfigIDByModelID.get(model)) || nilUUID;
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}
		const createdChat = await createMutation.mutateAsync({
			content,
			workspace_id: workspaceId,
			model_config_id: modelConfigID,
		});

		if (typeof window !== "undefined") {
			if (modelConfigID !== nilUUID) {
				localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}

		navigate(`/agents/${createdChat.id}`);
	};

	const handleOpenAnalytics = () => navigate("/agents/analytics");

	return (
		<>
			<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
				<NavLink to="/workspaces" className="inline-flex shrink-0 md:hidden">
					{logoUrl ? (
						<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
					) : (
						<CoderIcon className="h-6 w-6 fill-content-primary" />
					)}
				</NavLink>
				{isSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onExpandSidebar}
						aria-label="Expand sidebar"
						className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
					>
						<PanelLeftIcon />
					</Button>
				)}
				<div className="flex min-w-0 flex-1 items-center" />
				<div className="flex items-center gap-2">
					<ChimeButton />
					<WebPushButton />
				</div>
			</div>
			<AgentCreateForm
				onCreateChat={handleCreateChat}
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				modelCatalog={chatModelsQuery.data}
				modelOptions={catalogModelOptions}
				modelConfigs={chatModelConfigsQuery.data ?? EMPTY_MODEL_CONFIGS}
				isModelCatalogLoading={chatModelsQuery.isLoading}
				isModelConfigsLoading={chatModelConfigsQuery.isLoading}
				modelCatalogError={chatModelsQuery.error}
				onOpenAnalytics={handleOpenAnalytics}
			/>
		</>
	);
};

export default AgentCreatePage;
