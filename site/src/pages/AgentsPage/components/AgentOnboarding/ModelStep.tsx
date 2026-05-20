import { CheckIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { createChatModelConfig } from "#/api/queries/chats";
import type { ChatModelConfig, ChatProviderConfig } from "#/api/typesGenerated";
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
import {
	findKnownModelByExactAlias,
	getKnownModelsForProvider,
} from "../ChatModelAdminPanel/knownModels";
import { ProviderIcon } from "../ChatModelAdminPanel/ProviderIcon";

interface ModelStepProps {
	savedProviders: readonly ChatProviderConfig[];
	savedModels: readonly ChatModelConfig[];
	onBack: () => void;
	onSkip: () => void;
	onContinue: () => void;
}

export const ModelStep: FC<ModelStepProps> = ({
	savedProviders: allProviderConfigs,
	savedModels,
	onBack,
	onSkip,
	onContinue,
}) => {
	const savedProviders = allProviderConfigs.filter(
		(p) => p.source !== "supported" && p.enabled,
	);
	const [selectedProvider, setSelectedProvider] = useState<string>("");
	const [selectedModel, setSelectedModel] = useState<string>("");
	const [contextLimit, setContextLimit] = useState<string>("");
	const queryClient = useQueryClient();

	const createMutation = useMutation(createChatModelConfig(queryClient));

	const knownModels = useMemo(() => {
		if (!selectedProvider) return [];
		return getKnownModelsForProvider(selectedProvider);
	}, [selectedProvider]);

	const handleProviderChange = (value: string) => {
		setSelectedProvider(value);
		setSelectedModel("");
		setContextLimit("");
	};

	const handleModelChange = (value: string) => {
		setSelectedModel(value);
		// Auto-fill context limit from known model defaults
		const known = findKnownModelByExactAlias(selectedProvider, value);
		if (known?.contextLimit) {
			setContextLimit(String(known.contextLimit));
		}
	};

	const handleAddModel = () => {
		if (!selectedProvider || !selectedModel || !contextLimit) return;
		const limit = Number.parseInt(contextLimit, 10);
		if (Number.isNaN(limit) || limit <= 0) return;
		createMutation.mutate(
			{
				provider: selectedProvider,
				model: selectedModel,
				enabled: true,
				context_limit: limit,
			},
			{
				onSuccess: () => {
					setSelectedProvider("");
					setSelectedModel("");
					setContextLimit("");
				},
			},
		);
	};

	const hasSavedModels = savedModels.length > 0;
	const canAdd =
		selectedProvider &&
		selectedModel &&
		contextLimit &&
		!createMutation.isPending;

	return (
		<div className="flex flex-col gap-6">
			<div className="flex flex-col gap-3">
				<h2 className="text-2xl font-semibold">Add a model</h2>
				<p className="text-sm text-content-secondary">
					Set up at least one model to get started. For cost tracking,
					reasoning, and other advanced model options, go to Settings &rarr;{" "}
					<Link
						to="/agents/settings/models"
						className="text-content-link hover:text-content-link/80"
					>
						Models
					</Link>
					.
				</p>
			</div>

			{/* Saved models list */}
			{hasSavedModels && (
				<div className="flex flex-col gap-2">
					{savedModels.map((m) => (
						<div key={m.id} className="flex items-center gap-2 text-sm">
							<ProviderIcon provider={m.provider} className="size-5" />
							<span>
								{m.model} ({m.context_limit.toLocaleString()} context limit)
							</span>
							<CheckIcon className="size-4 text-green-500" />
						</div>
					))}
				</div>
			)}

			{/* Add model form */}
			<div className="flex flex-col gap-4 rounded-lg border border-border-default p-4">
				<div className="grid grid-cols-1 gap-4 md:grid-cols-3">
					<div className="flex flex-col gap-2">
						<Label className="text-sm font-medium">Model provider*</Label>
						<Select
							value={selectedProvider}
							onValueChange={handleProviderChange}
						>
							<SelectTrigger>
								<SelectValue placeholder="Select a provider..." />
							</SelectTrigger>
							<SelectContent>
								{savedProviders.map((p) => (
									<SelectItem key={p.id} value={p.provider}>
										<div className="flex items-center gap-2">
											<ProviderIcon provider={p.provider} className="size-5" />
											{formatProviderLabel(p.provider)}
										</div>
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>

					<div className="flex flex-col gap-2">
						<Label className="text-sm font-medium">Model identifier*</Label>
						{knownModels.length > 0 ? (
							<Select value={selectedModel} onValueChange={handleModelChange}>
								<SelectTrigger>
									<SelectValue placeholder="Select a model..." />
								</SelectTrigger>
								<SelectContent>
									{knownModels.map((m) => (
										<SelectItem
											key={m.modelIdentifier}
											value={m.modelIdentifier}
										>
											{m.displayName}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						) : (
							<Input
								placeholder="Enter model identifier..."
								value={selectedModel}
								onChange={(e) => setSelectedModel(e.target.value)}
							/>
						)}
					</div>

					<div className="flex flex-col gap-2">
						<Label className="text-sm font-medium">Context limit*</Label>
						<Input
							type="text"
							placeholder=""
							value={contextLimit}
							onChange={(e) => setContextLimit(e.target.value)}
						/>
					</div>
				</div>

				<div className="flex justify-end">
					<Button onClick={handleAddModel} disabled={!canAdd}>
						{createMutation.isPending && <Spinner className="mr-2" />}
						Add model
					</Button>
				</div>
			</div>

			{createMutation.isError && (
				<p className="text-sm text-content-destructive">
					Failed to add model. Please try again.
				</p>
			)}

			<div className="flex items-center justify-between">
				<button
					type="button"
					onClick={onBack}
					className="text-sm text-content-secondary transition-colors hover:text-content-primary"
				>
					Back
				</button>
				<div className="flex items-center gap-3">
					<Button variant="outline" onClick={onSkip}>
						Skip
					</Button>
					<Button onClick={onContinue} disabled={!hasSavedModels}>
						Continue
					</Button>
				</div>
			</div>
		</div>
	);
};
