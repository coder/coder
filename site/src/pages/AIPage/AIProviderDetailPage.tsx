import {
	ArrowLeftIcon,
	EllipsisVerticalIcon,
	GripVerticalIcon,
	PlusIcon,
	XIcon,
} from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { type FC, useCallback, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQueryClient } from "react-query";
import { createChatProviderConfig } from "#/api/queries/chats";
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
}

let nextId = 1;
const makeEmptyRow = (): ApiKeyRow => ({
	id: `key-${nextId++}`,
	name: "",
	apiKey: "",
	trackingId: "",
	updated: "",
});

const AIProviderDetailPage: FC = () => {
	const { providerType } = useParams<{ providerType: string }>();
	const provider = providerType ?? "anthropic";
	const label = formatProviderLabel(provider);
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const [apiKeys, setApiKeys] = useState<ApiKeyRow[]>(() => [makeEmptyRow()]);
	const [baseUrl, setBaseUrl] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	const createMutation = useMutation(createChatProviderConfig(queryClient));

	const addRow = useCallback(() => {
		setApiKeys((prev) => [...prev, makeEmptyRow()]);
	}, []);

	const removeRow = useCallback((id: string) => {
		setApiKeys((prev) => prev.filter((row) => row.id !== id));
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

	const handleAddProvider = useCallback(async () => {
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

	return (
		<div>
			<Link
				to="/ai/providers"
				className="inline-flex items-center gap-1 text-sm text-content-secondary no-underline hover:text-content-primary mb-6"
			>
				<ArrowLeftIcon className="size-4" />
				Back to providers
			</Link>

			<div className="flex items-center gap-3 mb-2">
				<ProviderIcon provider={provider} className="h-10 w-10 shrink-0" />
				<h1 className="text-3xl font-semibold m-0">{label}</h1>
			</div>
			<p className="text-content-secondary text-sm mt-0 mb-8">
				Connect third-party LLM services like OpenAI, Anthropic, or
				Google. Each provider supplies models that users can select for
				their conversations.
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
										<Input
											placeholder="Describe your key"
											value={row.name}
											onChange={(e) =>
												updateRow(row.id, "name", e.target.value)
											}
										/>
									</TableCell>
									<TableCell>
										<Input
											placeholder="Enter key"
											type="password"
											value={row.apiKey}
											onChange={(e) =>
												updateRow(row.id, "apiKey", e.target.value)
											}
										/>
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
										{i > 0 ? (
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
													<DropdownMenuItem
														className="gap-2 text-content-destructive"
														onClick={() => removeRow(row.id)}
													>
														<XIcon className="size-4" />
														Remove
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
				<Button disabled={!hasAnyKey || isSaving} onClick={handleAddProvider}>
					{isSaving ? "Adding..." : "Add provider"}
				</Button>
			</div>
		</div>
	);
};

export default AIProviderDetailPage;
