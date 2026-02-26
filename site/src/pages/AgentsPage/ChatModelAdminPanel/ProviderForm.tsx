import { getErrorMessage } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { CollapsibleContent } from "components/Collapsible/Collapsible";
import { Input } from "components/Input/Input";
import { Loader2Icon } from "lucide-react";
import { type FC, type FormEvent, useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { readOptionalString } from "./helpers";

type ProviderFormProps = {
	provider: string;
	providerConfig: TypesGen.ChatProviderConfig | undefined;
	baseURL: string;
	isEnvPreset: boolean;
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<unknown>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
};

export const ProviderForm: FC<ProviderFormProps> = ({
	provider,
	providerConfig,
	baseURL,
	isEnvPreset,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
}) => {
	const displayNameInputId = useId();
	const apiKeyInputId = useId();
	const baseURLInputId = useId();

	const [displayName, setDisplayName] = useState("");
	const [apiKey, setApiKey] = useState("");
	const [baseURLValue, setBaseURLValue] = useState("");

	useEffect(() => {
		setDisplayName(readOptionalString(providerConfig?.display_name) ?? "");
		setApiKey("");
		setBaseURLValue(baseURL);
	}, [providerConfig, baseURL]);

	const isAPIKeyEnvManaged = isEnvPreset && !providerConfig;
	const requiresAPIKey = !providerConfig && !isAPIKeyEnvManaged;
	const canSave =
		!providerConfigsUnavailable &&
		!isProviderMutationPending &&
		!isAPIKeyEnvManaged &&
		(!requiresAPIKey || apiKey.trim());

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (
			providerConfigsUnavailable ||
			isProviderMutationPending ||
			isAPIKeyEnvManaged
		) {
			return;
		}

		const trimmedDisplayName = displayName.trim();
		const trimmedAPIKey = apiKey.trim();
		const trimmedBaseURL = baseURLValue.trim();

		try {
			if (providerConfig) {
				const currentDisplayName =
					readOptionalString(providerConfig.display_name) ?? "";
				const currentBaseURL = baseURL.trim();
				const req: TypesGen.UpdateChatProviderConfigRequest = {
					...(trimmedDisplayName !== currentDisplayName && {
						display_name: trimmedDisplayName,
					}),
					...(trimmedAPIKey && { api_key: trimmedAPIKey }),
					...(trimmedBaseURL !== currentBaseURL && {
						base_url: trimmedBaseURL,
					}),
				};

				if (!req.display_name && !req.api_key && !req.base_url) {
					return;
				}

				await onUpdateProvider(providerConfig.id, req);
			} else {
				if (!trimmedAPIKey) {
					return;
				}

				const req: TypesGen.CreateChatProviderConfigRequest = {
					provider,
					api_key: trimmedAPIKey,
					...(trimmedDisplayName && {
						display_name: trimmedDisplayName,
					}),
					...(trimmedBaseURL && { base_url: trimmedBaseURL }),
				};

				await onCreateProvider(req);
			}

			// Only clear the API key field on success.
			setApiKey("");
		} catch (error) {
			toast.error(
				getErrorMessage(error, "Failed to save provider configuration."),
			);
		}
	};

	return (
		<CollapsibleContent className="border-t border-border px-5 py-4">
			<div className="space-y-3">
				<p className="m-0 text-xs text-content-secondary">
					{providerConfig
						? "Update this managed provider config for your deployment."
						: isAPIKeyEnvManaged
							? "This provider API key is managed by an environment variable."
							: "Create a managed provider config before enabling models."}
				</p>

				{isAPIKeyEnvManaged && (
					<Alert severity="info">
						<AlertTitle>API key managed by environment variable.</AlertTitle>
						<AlertDetail>
							This provider key is configured from deployment environment
							settings and cannot be edited in this UI.
						</AlertDetail>
					</Alert>
				)}

				{!isAPIKeyEnvManaged && (
					<form
						className="space-y-3"
						onSubmit={(event) => void handleSubmit(event)}
					>
						<div className="grid gap-3 lg:grid-cols-3">
							<div className="grid gap-1.5">
								<label
									htmlFor={displayNameInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Display name
								</label>
								<Input
									id={displayNameInputId}
									className="h-10 text-[13px]"
									placeholder="Friendly provider label"
									value={displayName}
									onChange={(e) => setDisplayName(e.target.value)}
									disabled={
										providerConfigsUnavailable || isProviderMutationPending
									}
								/>
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={apiKeyInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									API key{" "}
									{!providerConfig && (
										<span className="text-xs text-content-destructive font-bold">
											*
										</span>
									)}
								</label>
								<Input
									id={apiKeyInputId}
									type="password"
									autoComplete="off"
									className="h-10 text-[13px]"
									placeholder={
										providerConfig
											? "Leave blank to keep existing key"
											: "Paste provider API key"
									}
									value={apiKey}
									onChange={(e) => setApiKey(e.target.value)}
									disabled={
										providerConfigsUnavailable || isProviderMutationPending
									}
								/>
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={baseURLInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Base URL
								</label>
								<Input
									id={baseURLInputId}
									className="h-10 text-[13px]"
									placeholder="https://api.example.com/v1"
									value={baseURLValue}
									onChange={(e) => setBaseURLValue(e.target.value)}
									disabled={
										providerConfigsUnavailable || isProviderMutationPending
									}
								/>
							</div>
						</div>
						<div className="flex items-center justify-end gap-3 border-t border-border pt-3">
							<Button size="sm" type="submit" disabled={!canSave}>
								{isProviderMutationPending && (
									<Loader2Icon className="h-4 w-4 animate-spin" />
								)}
								{providerConfig ? "Save changes" : "Create provider config"}
							</Button>
						</div>
					</form>
				)}
			</div>
		</CollapsibleContent>
	);
};
