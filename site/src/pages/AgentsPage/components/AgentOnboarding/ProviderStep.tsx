import { ExternalLinkIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { createChatProviderConfig } from "#/api/queries/chats";
import type { ChatProviderConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { formatProviderLabel } from "../../utils/modelOptions";
import { ProviderIcon } from "../ChatModelAdminPanel/ProviderIcon";

const KNOWN_PROVIDERS = [
	"anthropic",
	"openai",
	"google",
	"azure",
	"bedrock",
	"openrouter",
] as const;

const API_KEY_URLS: Record<string, string> = {
	anthropic: "https://console.anthropic.com/settings/keys",
	openai: "https://platform.openai.com/api-keys",
	google: "https://aistudio.google.com/apikey",
	azure: "https://portal.azure.com",
	openrouter: "https://openrouter.ai/settings/keys",
};

interface ProviderStepProps {
	savedProviders: readonly ChatProviderConfig[];
	onSkip: () => void;
	onContinue: () => void;
}

export const ProviderStep: FC<ProviderStepProps> = ({
	savedProviders: allProviderConfigs,
	onSkip,
	onContinue,
}) => {
	// Filter to only providers that are actually configured (not just
	// catalog "supported" entries).
	const savedProviders = allProviderConfigs.filter(
		(p) => p.source !== "supported" && p.enabled,
	);
	const [started, setStarted] = useState(savedProviders.length > 0);
	const [selectedProvider, setSelectedProvider] = useState<string>("");
	const [apiKey, setApiKey] = useState("");
	const [baseUrl, setBaseUrl] = useState("");
	const queryClient = useQueryClient();

	const createMutation = useMutation(createChatProviderConfig(queryClient));

	const handleSave = () => {
		if (!selectedProvider) return;
		createMutation.mutate(
			{
				provider: selectedProvider,
				api_key: apiKey || undefined,
				base_url: baseUrl || undefined,
				enabled: true,
				central_api_key_enabled: true,
			},
			{
				onSuccess: () => {
					setSelectedProvider("");
					setApiKey("");
					setBaseUrl("");
				},
			},
		);
	};

	const hasSavedProviders = savedProviders.length > 0;
	const savedProviderIds = new Set(
		savedProviders.map((p) => p.provider.toLowerCase()),
	);
	const availableProviders = KNOWN_PROVIDERS.filter(
		(p) => !savedProviderIds.has(p),
	);

	// Intro sub-state: before the user clicks "Get started"
	if (!started) {
		return (
			<div className="flex min-h-[460px] flex-col gap-6">
				<div className="flex flex-col gap-4">
					<h2 className="text-2xl font-semibold">Welcome to Coder Agents.</h2>
					<p className="text-base text-content-secondary">
						Let's get you set up so you can start building.
					</p>
				</div>

				<ol className="flex list-decimal flex-col gap-1 pl-5 text-base text-content-secondary">
					<li>Set up at least one provider</li>
					<li>Add a model</li>
					<li>Start chatting</li>
				</ol>

				<div className="mt-auto flex items-center justify-end gap-3">
					<Button variant="outline" onClick={onSkip}>
						Skip
					</Button>
					<Button onClick={() => setStarted(true)}>Get started</Button>
				</div>
			</div>
		);
	}

	// Configuring sub-state
	return (
		<div className="flex flex-col gap-6">
			<div className="flex flex-col gap-3">
				<h2 className="text-2xl font-semibold">Connect an AI provider.</h2>
				<p className="text-sm text-content-secondary">
					You'll need to set up at least one provider to get started.
					<br />
					For key policy and API key controls, go to Settings &rarr;{" "}
					<Link
						to="/agents/settings/api-keys"
						className="text-content-link hover:text-content-link/80"
					>
						Advanced
					</Link>
					.
				</p>
			</div>

			{/* Saved provider badges */}
			{hasSavedProviders && (
				<div className="flex flex-wrap items-center gap-2">
					{savedProviders.map((p) => (
						<div
							key={p.id}
							className="flex items-center gap-2 rounded-lg border-2 border-green-500 px-3 py-1.5"
						>
							<ProviderIcon provider={p.provider} className="size-5" />
							<span className="text-sm font-medium">
								{formatProviderLabel(p.provider)}
							</span>
						</div>
					))}
					{availableProviders.length > 0 && !selectedProvider && (
						<Select value="" onValueChange={setSelectedProvider}>
							<SelectTrigger className="w-auto gap-2">
								<SelectValue placeholder="Add another..." />
							</SelectTrigger>
							<SelectContent>
								{availableProviders.map((p) => (
									<SelectItem key={p} value={p}>
										<div className="flex items-center gap-2">
											<ProviderIcon provider={p} className="size-5" />
											{formatProviderLabel(p)}
										</div>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					)}
				</div>
			)}

			{/* Provider selector (when no providers saved yet) */}
			{!hasSavedProviders && !selectedProvider && (
				<Select value="" onValueChange={setSelectedProvider}>
					<SelectTrigger className="w-[200px]">
						<SelectValue placeholder="Select..." />
					</SelectTrigger>
					<SelectContent>
						{availableProviders.map((p) => (
							<SelectItem key={p} value={p}>
								<div className="flex items-center gap-2">
									<ProviderIcon provider={p} className="size-5" />
									{formatProviderLabel(p)}
								</div>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			)}

			{/* Provider config form */}
			{selectedProvider ? (
				<div className="flex flex-col gap-4 rounded-xl border border-solid border-border p-6">
					<div className="flex items-center justify-between">
						<Label className="text-sm font-medium">
							{formatProviderLabel(selectedProvider)} API key
						</Label>
						{API_KEY_URLS[selectedProvider] && (
							<a
								href={API_KEY_URLS[selectedProvider]}
								target="_blank"
								rel="noreferrer"
								className="inline-flex items-center gap-1 text-sm text-content-link hover:text-content-link/80"
							>
								Get API key
								<ExternalLinkIcon className="size-3" />
							</a>
						)}
					</div>
					<Input
						type="text"
						className="[-webkit-text-security:disc]"
						autoComplete="off"
						data-1p-ignore
						data-lpignore="true"
						placeholder="sk-..."
						value={apiKey}
						onChange={(e) => setApiKey(e.target.value)}
					/>

					<Label className="text-sm font-medium">Base URL</Label>
					<Input
						type="text"
						placeholder={
							selectedProvider === "anthropic" ||
							selectedProvider === "bedrock" ||
							selectedProvider === "google"
								? "https://api.example.com"
								: "https://api.example.com/v1"
						}
						value={baseUrl}
						onChange={(e) => setBaseUrl(e.target.value)}
					/>

					<div className="flex justify-end">
						<Button onClick={handleSave} disabled={!apiKey.trim() || createMutation.isPending}>
							{createMutation.isPending && <Spinner className="mr-2" />}
							Save provider
						</Button>
					</div>
				</div>
			) : (
				<div className="flex min-h-[200px] items-center justify-center rounded-xl border border-solid border-border p-6">
					<p className="text-sm text-content-secondary">
						{hasSavedProviders ? "Select another provider" : "No provider selected"}
					</p>
				</div>
			)}

			{createMutation.isError && (
				<p className="text-sm text-content-destructive">
					Failed to save provider. Please try again.
				</p>
			)}

			<div className="flex items-center justify-between">
				<Button
					variant="subtle"
					className="min-w-0 px-0"
					onClick={() => {
						setSelectedProvider("");
						setApiKey("");
						setBaseUrl("");
						setStarted(false);
					}}
				>
					Back
				</Button>
				<div className="flex items-center gap-3">
					<Button variant="outline" onClick={onSkip}>
						Skip
					</Button>
					<Button onClick={onContinue} disabled={!hasSavedProviders}>
						Continue
					</Button>
				</div>
			</div>
		</div>
	);
};
