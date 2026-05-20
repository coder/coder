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
import { type FC, useCallback, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
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
import { cn } from "#/utils/cn";

interface ApiKeyRow {
	id: string;
	name: string;
	apiKey: string;
	trackingId: string;
	updated: string;
	/** Whether this key was loaded from the server (saved). */
	saved: boolean;
	/** Whether the user clicked "Edit API key" on a saved row. */
	editing: boolean;
}

let nextId = 1;
const makeEmptyRow = (): ApiKeyRow => ({
	id: `key-${nextId++}`,
	name: "",
	apiKey: "",
	trackingId: "",
	updated: "",
	saved: false,
	editing: false,
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

	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCount = useMemo(() => {
		const configs = modelConfigsQuery.data ?? [];
		return configs.filter((mc) => mc.provider === provider).length;
	}, [modelConfigsQuery.data, provider]);

	const isEditMode = !!existingConfig;

	// Seed state from existing config when it loads.
	const [seeded, setSeeded] = useState(false);
	const [apiKeys, setApiKeys] = useState<ApiKeyRow[]>(() => [makeEmptyRow()]);
	const [baseUrl, setBaseUrl] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	if (existingConfig && !seeded) {
		setSeeded(true);
		setBaseUrl(existingConfig.base_url ?? "");
		if (existingConfig.has_api_key) {
			setApiKeys([
				{
					id: `saved-${existingConfig.id}`,
					name: existingConfig.display_name || "API key",
					apiKey: "sk-\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022",
					trackingId: `key_${existingConfig.id.slice(0, 16)}`,
					updated: existingConfig.updated_at
						? `${Math.round((Date.now() - new Date(existingConfig.updated_at).getTime()) / 86400000)} days ago`
						: "",
					saved: true,
					editing: false,
				},
			]);

		}
	}


	const createMutation = useMutation(createChatProviderConfig(queryClient));
	const deleteMutation = useMutation(deleteChatProviderConfig(queryClient));

	const addRow = useCallback(() => {
		setApiKeys((prev) => [...prev, makeEmptyRow()]);
	}, []);

	// Drag-to-reorder state.
	const [draggingIdx, setDraggingIdx] = useState<number | null>(null);
	const dragIdx = useRef<number | null>(null);
	const [dragOverIdx, setDragOverIdx] = useState<number | null>(null);

	const handleDragStart = useCallback((i: number) => {
		dragIdx.current = i;
		setDraggingIdx(i);
	}, []);

	const handleDragOver = useCallback(
		(e: React.DragEvent, i: number) => {
			e.preventDefault();
			if (dragOverIdx !== i) {
				setDragOverIdx(i);
			}
		},
		[dragOverIdx],
	);

	const handleDrop = useCallback(
		(i: number) => {
			const from = dragIdx.current;
			if (from === null || from === i) {
				dragIdx.current = null;
				setDragOverIdx(null);
				setDraggingIdx(null);
				return;
			}
			setApiKeys((prev) => {
				const next = [...prev];
				const [moved] = next.splice(from, 1);
				next.splice(i, 0, moved);
				return next;
			});
			dragIdx.current = null;
			setDragOverIdx(null);
			setDraggingIdx(null);
		},
		[],
	);

	const handleDragEnd = useCallback(() => {
		dragIdx.current = null;
		setDragOverIdx(null);
		setDraggingIdx(null);
	}, []);

	const removeRow = useCallback(
		(id: string) => {
			setApiKeys((prev) => {
				const filtered = prev.filter((row) => row.id !== id);
				if (filtered.length === 0) {
					return [makeEmptyRow()];
				}
				return filtered;
			});
		},
		[],
	);

	const editRow = useCallback((id: string) => {
		setApiKeys((prev) =>
			prev.map((row) =>
				row.id === id ? { ...row, editing: true } : row,
			),
		);
	}, []);

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
						{" "}You have {modelCount === 0 ? "no" : modelCount} {modelCount === 1 ? "model" : "models"} added for this provider.{" "}
						<Link
								to={modelCount === 0
									? `/ai/models?filterProvider=${provider}&newModel=${provider}`
									: `/ai/models?filterProvider=${provider}`
								}
								className="text-content-link"
							>
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
								<TableRow
									key={row.id}
									draggable
									onDragStart={() => handleDragStart(i)}
									onDragOver={(e) => handleDragOver(e, i)}
									onDrop={() => handleDrop(i)}
									onDragEnd={handleDragEnd}
									className={cn(
										"transition-all duration-150",
										draggingIdx === i && "opacity-40",
										dragOverIdx === i && draggingIdx !== i && "border-t-2 border-t-content-link bg-surface-secondary/50",

									)}
								>
									<TableCell>
										<GripVerticalIcon className="size-4 text-content-disabled cursor-grab active:cursor-grabbing" />
									</TableCell>
									<TableCell>
										{row.saved && !row.editing ? (
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
										{row.saved && !row.editing ? (
											<span className="text-sm text-content-secondary font-mono">
												{row.apiKey}
											</span>
										) : row.saved ? (
											<Input
												value={row.apiKey}
												readOnly
												tabIndex={-1}
												autoComplete="off"
												className="font-mono select-none pointer-events-none"
												onCopy={(e) => e.preventDefault()}
											/>
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
															<DropdownMenuItem className="gap-2" onClick={() => editRow(row.id)}>
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
