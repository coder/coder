import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { ChevronRightIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { cn } from "utils/cn";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ProviderForm } from "./ProviderForm";
import { ProviderIcon } from "./ProviderIcon";

const getProviderModelsLabel = (providerState: ProviderState): string => {
	if (providerState.modelConfigs.length > 0) {
		return `${providerState.modelConfigs.length} configured model${providerState.modelConfigs.length === 1 ? "" : "s"}`;
	}
	if (providerState.catalogModelCount > 0) {
		return `${providerState.catalogModelCount} catalog model${providerState.catalogModelCount === 1 ? "" : "s"}`;
	}
	return "No models configured";
};

type ProvidersSectionProps = {
	providerStates: readonly ProviderState[];
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<unknown>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onSelectedProviderChange: (provider: string) => void;
};

export const ProvidersSection: FC<ProvidersSectionProps> = ({
	providerStates,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onSelectedProviderChange,
}) => {
	const [expandedProvider, setExpandedProvider] = useState<string | null>(null);

	// Reset expanded provider when available providers change.
	useEffect(() => {
		setExpandedProvider((current) => {
			if (current && providerStates.some((ps) => ps.provider === current)) {
				return current;
			}
			return null;
		});
	}, [providerStates]);

	if (providerStates.length === 0) {
		return (
			<div className="rounded-lg border border-dashed border-border bg-surface-primary p-4 text-[13px] text-content-secondary">
				No provider types were returned by the backend.
			</div>
		);
	}

	return (
		<div className="space-y-2">
			{providerStates.map((providerState) => {
				const isExpanded = expandedProvider === providerState.provider;
				const modelsLabel = getProviderModelsLabel(providerState);

				return (
					<Collapsible
						key={providerState.provider}
						open={isExpanded}
						onOpenChange={(nextOpen) => {
							setExpandedProvider(nextOpen ? providerState.provider : null);
							if (nextOpen) {
								onSelectedProviderChange(providerState.provider);
							}
						}}
					>
						<div
							className={cn(
								"rounded-xl border border-border-default bg-surface-primary shadow-sm transition-all",
								isExpanded &&
									"border-border-default bg-surface-secondary/30 shadow-md",
							)}
						>
							<CollapsibleTrigger asChild>
								<Button
									variant="subtle"
									className={cn(
										"h-auto w-full justify-between gap-4 rounded-[inherit] px-5 py-3.5 text-left shadow-none",
										isExpanded
											? "bg-surface-secondary/30 hover:bg-surface-secondary/30"
											: "hover:bg-surface-tertiary/30",
									)}
								>
									<div className="flex min-w-0 items-center gap-3">
										<ProviderIcon
											provider={providerState.provider}
											className="h-7 w-7"
											active={providerState.hasEffectiveAPIKey}
										/>
										<div className="min-w-0">
											<span
												className={cn(
													"truncate text-[15px] font-semibold",
													providerState.hasEffectiveAPIKey
														? "text-content-primary"
														: "text-content-secondary",
												)}
											>
												{providerState.label}
											</span>
											<div className="mt-0.5 flex items-center gap-2 text-xs text-content-secondary">
												<span className="truncate">{modelsLabel}</span>
											</div>
										</div>
									</div>
									<ChevronRightIcon
										className={cn(
											"h-4 w-4 shrink-0 text-content-secondary transition-transform duration-200",
											isExpanded && "rotate-90 text-content-primary",
										)}
									/>
								</Button>
							</CollapsibleTrigger>

							{isExpanded && (
								<ProviderForm
									provider={providerState.provider}
									providerConfig={providerState.providerConfig}
									baseURL={providerState.baseURL}
									isEnvPreset={providerState.isEnvPreset}
									providerConfigsUnavailable={providerConfigsUnavailable}
									isProviderMutationPending={isProviderMutationPending}
									onCreateProvider={onCreateProvider}
									onUpdateProvider={onUpdateProvider}
								/>
							)}
						</div>
					</Collapsible>
				);
			})}
		</div>
	);
};
