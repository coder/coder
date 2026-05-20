import { ExternalLinkIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { createChatProviderConfig } from "#/api/queries/chats";
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
	onSkip: () => void;
	onContinue: () => void;
}

export const ProviderStep: FC<ProviderStepProps> = ({ onSkip, onContinue }) => {
	const [started, setStarted] = useState(false);
	const [selectedProvider, setSelectedProvider] = useState<string>("");
	const [apiKey, setApiKey] = useState("");
	const [baseUrl, setBaseUrl] = useState("");
	const [_providerSaved, setProviderSaved] = useState(false);
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
					onContinue();
				},
			},
		);
	};

	// Intro sub-state: before the user clicks "Get started"
	if (!started) {
		return (
			<div className="flex h-full flex-col gap-4">
				<div className="flex flex-col gap-1">
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
		<div className="flex h-full flex-col gap-4">
			<div className="flex flex-col gap-1">
				<h2 className="text-2xl font-semibold">Connect an AI provider.</h2>
				<p className="text-sm text-content-secondary">
					You'll need to set up at least one provider to get started.
					<br />
					For key policy and API key controls, go to Settings &rarr;{" "}
					<Link
						to="/agents/settings/providers"
						className="text-content-link hover:text-content-link/80"
					>
						Providers
					</Link>
					.
				</p>
			</div>

			{/* Provider selector */}
			{!selectedProvider && (
				<Select value="" onValueChange={setSelectedProvider}>
					<SelectTrigger className="w-[200px]">
						<SelectValue placeholder="Select..." />
					</SelectTrigger>
					<SelectContent>
						{KNOWN_PROVIDERS.map((p) => (
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
					<div className="flex flex-col gap-1.5">
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
						</div>

					<div className="flex flex-col gap-1.5">
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
						</div>
				</div>
			) : (
				<div className="flex min-h-[200px] items-center justify-center rounded-xl border border-solid border-border p-6">
					<p className="text-sm text-content-secondary">No provider selected</p>
				</div>
			)}

			{createMutation.isError && (
				<p className="text-sm text-content-destructive">
					Failed to save provider. Please try again.
				</p>
			)}

			<div className="mt-auto flex items-center justify-between">
				<Button
					variant="subtle"
					className="min-w-0 px-0"
					onClick={() => {
						setSelectedProvider("");
						setApiKey("");
						setBaseUrl("");
						setProviderSaved(false);
					}}
				>
					Back
				</Button>
				<div className="flex items-center gap-3">
					<Button variant="outline" onClick={onSkip}>
						Skip
					</Button>
					<Button
						onClick={handleSave}
						disabled={
							!selectedProvider || !apiKey.trim() || createMutation.isPending
						}
					>
						{createMutation.isPending && <Spinner className="mr-2" />}
						Continue
					</Button>
				</div>
			</div>
		</div>
	);
};
