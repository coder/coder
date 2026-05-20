import { ArrowLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProviderConfigs,
	createChatProviderConfig,
	deleteChatProviderConfig,
} from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { ProviderIcon } from "../AgentsPage/components/ChatModelAdminPanel/ProviderIcon";

const AIProviderBedrockPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const existingConfig = providerConfigsQuery.data?.find(
		(pc) =>
			pc.provider === "bedrock" &&
			(pc.source === "database" || pc.source === "env_preset"),
	);
	const isEditMode = !!existingConfig;

	const [baseUrl, setBaseUrl] = useState("");
	const [model, setModel] = useState("");
	const [smallFastModel, setSmallFastModel] = useState("");
	const [accessKey, setAccessKey] = useState("");
	const [accessKeySecret, setAccessKeySecret] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	// Seed from existing config.
	const [seeded, setSeeded] = useState(false);
	if (existingConfig && !seeded) {
		setSeeded(true);
		setBaseUrl(existingConfig.base_url ?? "");
	}

	const createMutation = useMutation(createChatProviderConfig(queryClient));
	const deleteMutation = useMutation(deleteChatProviderConfig(queryClient));

	const hasRequiredFields = accessKey.length > 0 && accessKeySecret.length > 0;

	const handleSave = async () => {
		setIsSaving(true);
		try {
			await createMutation.mutateAsync({
				provider: "bedrock",
				api_key: accessKey,
				base_url: baseUrl || undefined,
				enabled: true,
				central_api_key_enabled: true,
			});
			navigate("/ai/providers");
		} finally {
			setIsSaving(false);
		}
	};

	const handleDelete = async () => {
		if (!existingConfig) return;
		await deleteMutation.mutateAsync(existingConfig.id);
		navigate("/ai/providers");
	};

	return (
		<div>
			{/* Top bar */}
			<div className="flex items-center justify-between mb-6">
				<Link
					to="/ai/providers"
					className="inline-flex items-center gap-1 text-sm text-content-secondary no-underline hover:text-content-primary"
				>
					<ArrowLeftIcon className="size-4" />
					Back to providers
				</Link>
				{isEditMode && (
					<Button variant="destructive" onClick={handleDelete}>
						Delete
					</Button>
				)}
			</div>

			<div className="flex items-center gap-3 mb-2">
				<ProviderIcon provider="bedrock" className="h-10 w-10 shrink-0" />
				<h1 className="text-3xl font-semibold m-0">AWS Bedrock</h1>
			</div>
			<p className="text-content-secondary text-sm mt-0 mb-8">
				Connect third-party LLM services like OpenAI, Anthropic, or
				Google. Each provider supplies models that users can select for
				their conversations.
			</p>

			<div className="border border-solid border-border rounded-lg p-8">
				{/* Base URL */}
				<div className="mb-8">
					<h3 className="text-sm font-semibold text-content-primary mt-0 mb-1">
						Base URL
					</h3>
					<p className="text-sm text-content-secondary mt-0 mb-2">
						Custom endpoint for this provider. Leave empty to use the
						default.
					</p>
					<Input
						type="url"
						placeholder="https://api.anthropic.com/"
						value={baseUrl}
						onChange={(e) => setBaseUrl(e.target.value)}
						autoComplete="off"
					/>
				</div>

				{/* Model + Small-fast model */}
				<div className="grid grid-cols-2 gap-6 mb-8">
					<div>
						<h3 className="text-sm font-semibold text-content-primary mt-0 mb-2">
							Model
						</h3>
						<Input
							value={model}
							onChange={(e) => setModel(e.target.value)}
							autoComplete="off"
						/>
					</div>
					<div>
						<h3 className="text-sm font-semibold text-content-primary mt-0 mb-2">
							Small-fast model
						</h3>
						<Input
							value={smallFastModel}
							onChange={(e) => setSmallFastModel(e.target.value)}
							autoComplete="off"
						/>
					</div>
				</div>

				{/* Access key + Access key secret */}
				<div className="grid grid-cols-2 gap-6 mb-8">
					<div>
						<h3 className="text-sm font-semibold text-content-primary mt-0 mb-2">
							Access key
						</h3>
						<Input
							value={accessKey}
							onChange={(e) => setAccessKey(e.target.value)}
							autoComplete="off"
							data-1p-ignore
							data-lpignore="true"
						/>
					</div>
					<div>
						<h3 className="text-sm font-semibold text-content-primary mt-0 mb-2">
							Access key secret
						</h3>
						<Input
							value={accessKeySecret}
							onChange={(e) => setAccessKeySecret(e.target.value)}
							autoComplete="off"
							data-1p-ignore
							data-lpignore="true"
						/>
					</div>
				</div>

				{/* Actions inside the card */}
				<div className="flex items-center justify-end gap-3">
					<Button variant="outline" asChild>
						<Link to="/ai/providers">Cancel</Link>
					</Button>
					<Button
						disabled={(!hasRequiredFields && !isEditMode) || isSaving}
						onClick={handleSave}
					>
						{isSaving
							? "Saving..."
							: isEditMode
								? "Update provider"
								: "Add Provider"}
					</Button>
				</div>
			</div>
		</div>
	);
};

export default AIProviderBedrockPage;
