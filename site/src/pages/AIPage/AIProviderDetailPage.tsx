import {
	ArrowLeftIcon,
	EllipsisVerticalIcon,
	GripVerticalIcon,
	PencilIcon,
	PlusIcon,
	Trash2Icon,
} from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { type FC, useCallback, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProviderConfigs,
	createChatProviderConfig,
	deleteChatProviderConfig,
} from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { ProviderIcon } from "../AgentsPage/components/ChatModelAdminPanel/ProviderIcon";
import { formatProviderLabel } from "../AgentsPage/utils/modelOptions";

interface ApiKeyRow {
	id: string;
	name: string;
	apiKey: string;
	trackingId: string;
	updated: string;
	/** Whether this key was loaded from the server (saved). */
	saved: boolean;
}

let nextId = 1;
const makeEmptyRow = (): ApiKeyRow => ({
	id: `key-${nextId++}`,
	name: "",
	apiKey: "",
	trackingId: "",
	updated: "",
	saved: false,
});

const AIProviderDetailPage: FC = () => {
	const { providerType } = useParams<{ providerType: string }>();
	const provider = providerType ?? "anthropic";
	const label = formatProviderLabel(provider);
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	// Check if this provider is already configured.
	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const existingConfig = useMemo(() => {
		const configs = providerConfigsQuery.data ?? [];
		return configs.find(
			(pc) =>
				pc.provider === provider &&
				(pc.source === "database" || pc.source === "env_preset"),
		);
	}, [providerConfigsQuery.data, provider]);

	const isEditMode = !!existingConfig;

	const [apiKeys, setApiKeys] = useState<ApiKeyRow[]>(() => [makeEmptyRow()]);
	const [baseUrl, setBaseUrl] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	const createMutation = useMutation(createChatProviderConfig(queryClient));
	const deleteMutation = useMutation(deleteChatProviderConfig(queryClient));

	const addRow = useCallback(() => {
		setApiKeys((prev) => [...prev, makeEmptyRow()]);
	}, []);

	const removeRow = useCallback(
		(id: string) => {
			setApiKeys((prev) => {
				const filtered = prev.filter((row) => row.id !== id);
				// If removing the last row, reset to a single empty row.
				if (filtered.length === 0) {
					return [makeEmptyRow()];
				}
				return filtered;
			});
		},
		[],
	);

	const updateRow = useCallback(
		(id: string, field: keyof ApiKeyRow, value: string) => {
			setApiKeys((prev) =>
				prev.map((row) =>
					row.id === id ? { ...row, [field]: value } : row,
				),
			);
		},
		[],
	);

	const hasAnyKey = apiKeys.some((row) => row.apiKey.length > 0);

	const handleSave = useCallback(async () => {
		const firstKey = apiKeys.find((row) => row.apiKey.length > 0);
		if (!firstKey) return;
		setIsSaving(true);
		try {
			await createMutation.mutateAsync({
				provider,
				api_key: firstKey.apiKey,
				base_url: baseUrl || undefined,
				enabled: true,
				central_api_key_enabled: true,
			});
			navigate("/ai/providers");
		} finally {
			setIsSaving(false);
		}
	}, [apiKeys, baseUrl, provider, createMutation, navigate]);

	const handleDelete = useCallback(async () => {
		if (!existingConfig) return;
		await deleteMutation.mutateAsync(existingConfig.id);
		navigate("/ai/providers");
	}, [existingConfig, deleteMutation, navigate]);

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
				<ProviderIcon provider={provider} className="h-10 w-10 shrink-0" />
				<h1 className="text-3xl font-semibold m-0">{label}</h1>
			</div>
			<p className="text-content-secondary text-sm mt-0 mb-8">
				Connect third-party LLM services like OpenAI, Anthropic, or
				Google. Each provider supplies models that users can select for
				their conversations.
				{isEditMode && (
					<>
						{" "}You have {existingConfig?.display_name ? "" : ""}models added for this provider.{" "}
						<Link to="/ai/models" className="text-content-link">
							Manage models
						</Link>
					</>
				)}
			</p>

			<div className="border border-solid border-border rounded-lg p-8">
				{/* API key(s) */}
				<div className="mb-8">
					<h3 className="text-sm font-semibold text-content-primary mt-0 mb-1">
						API key(s)
					</h3>
					<p className="text-sm text-content-secondary mt-0 mb-4">
						Secret key used to authenticate requests to this provider.
						You can add more than one key. Coder Agents will default to
						the first key in the list.
					</p>

					<Table aria-label="API keys">
						<TableHeader>
							<TableRow>
								<TableHead className="w-8" />
								<TableHead>Name</TableHead>
								<TableHead>API key</TableHead>
								<TableHead>Tracking ID</TableHead>
								<TableHead>Updated</TableHead>
								<TableHead className="w-10" />
							</TableRow>
						</TableHeader>
						<TableBody>
							{apiKeys.map((row, i) => (
								<TableRow key={row.id}>
									<TableCell>
										<GripVerticalIcon className="size-4 text-content-disabled cursor-grab" />
									</TableCell>
									<TableCell>
										{row.saved ? (
											<span className="text-sm text-content-primary">
												{row.name}
											</span>
										) : (
											<Input
												placeholder="Describe your key"
												value={row.name}
												autoComplete="off"
												onChange={(e) =>
													updateRow(row.id, "name", e.target.value)
												}
											/>
										)}
									</TableCell>
									<TableCell>
										{row.saved ? (
											<span className="text-sm text-content-secondary font-mono">
												{row.apiKey}
											</span>
										) : (
											<Input
												placeholder="Enter key"
												type="text"
												autoComplete="off"
												data-1p-ignore
												data-lpignore="true"
												className="font-mono"
												value={row.apiKey}
												onChange={(e) =>
													updateRow(row.id, "apiKey", e.target.value)
												}
											/>
										)}
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-secondary">
											{row.trackingId}
										</span>
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-secondary">
											{row.updated}
										</span>
									</TableCell>
									<TableCell>
										{isEditMode || i > 0 ? (
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<button
														type="button"
														className="flex items-center justify-center w-8 h-8 rounded-md bg-transparent border-none cursor-pointer hover:bg-surface-secondary"
													>
														<EllipsisVerticalIcon className="size-4 text-content-secondary" />
													</button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													{row.saved && (
														<DropdownMenuItem className="gap-2">
															<PencilIcon className="size-4" />
															Edit API key
														</DropdownMenuItem>
													)}
													<DropdownMenuItem
														className="gap-2 text-content-destructive"
														onClick={() => removeRow(row.id)}
													>
														<Trash2Icon className="size-4" />
														Delete
													</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										) : (
											<div className="w-8 h-8" />
										)}
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>

					<Button variant="outline" onClick={addRow} className="mt-3">
						<PlusIcon className="size-4" />
						Add API key
					</Button>
				</div>

				{/* Base URL */}
				<div>
					<h3 className="text-sm font-semibold text-content-primary mt-0 mb-1">
						Base URL
					</h3>
					<p className="text-sm text-content-secondary mt-0 mb-2">
						Custom endpoint for this provider. Leave empty to use the
						default.
					</p>
					<Input
						type="url"
						placeholder={`https://api.${provider}.com/`}
						value={baseUrl}
						onChange={(e) => setBaseUrl(e.target.value)}
						className="max-w-lg"
					/>
				</div>
			</div>

			{/* Actions */}
			<div className="flex items-center justify-end gap-3 mt-6">
				<Button variant="outline" asChild>
					<Link to="/ai/providers">Cancel</Link>
				</Button>
				<Button disabled={!hasAnyKey || isSaving} onClick={handleSave}>
					{isSaving
						? "Saving..."
						: isEditMode
							? "Update provider"
							: "Add provider"}
				</Button>
			</div>
		</div>
	);
};

export default AIProviderDetailPage;
