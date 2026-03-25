import {
	CheckCircleIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { cn } from "utils/cn";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { SectionHeader } from "../SectionHeader";
import type { ProviderState } from "./ChatModelAdminPanel";
import { readOptionalString } from "./helpers";
import { ProviderForm } from "./ProviderForm";
import { ProviderIcon } from "./ProviderIcon";

type ProviderView =
	| { mode: "list" }
	| { mode: "detail"; configId: string }
	| { mode: "add"; provider: string };

const nilUUID = "00000000-0000-0000-0000-000000000000";

const isLocalDatabaseConfig = (pc: TypesGen.ChatProviderConfig): boolean => {
	if (pc.id === nilUUID) return false;
	return !pc.source || pc.source === "database";
};

const getProviderConfigs = (providerState: ProviderState) =>
	providerState.providerConfigs;

interface ProvidersSectionProps {
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
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
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	onSelectedProviderChange: (provider: string) => void;
}

export const ProvidersSection: FC<ProvidersSectionProps> = ({
	sectionLabel,
	sectionDescription,
	sectionBadge,
	providerStates,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	onSelectedProviderChange,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const navigate = useNavigate();
	const location = useLocation();
	const canGoBack =
		(location.state as { pushed?: boolean } | null)?.pushed === true;

	// Derive the current view from URL search params so that
	// browser back/forward navigation works as expected.
	const view: ProviderView = (() => {
		const configParam = searchParams.get("providerConfig");
		if (configParam) {
			const exists = providerStates.some((ps) =>
				ps.providerConfigs.some((pc) => pc.id === configParam),
			);
			return exists
				? { mode: "detail", configId: configParam }
				: { mode: "list" };
		}
		const newProviderParam = searchParams.get("newProvider");
		if (newProviderParam) {
			const exists = providerStates.some(
				(ps) => ps.provider === newProviderParam,
			);
			return exists
				? { mode: "add", provider: newProviderParam }
				: { mode: "list" };
		}
		return { mode: "list" };
	})();

	const clearProviderView = () => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.delete("providerConfig");
			next.delete("newProvider");
			return next;
		});
	};

	// ── Detail view ───────────────────────────────────────────
	if (view.mode === "detail") {
		let selectedConfig: TypesGen.ChatProviderConfig | undefined;
		let parentProviderState: ProviderState | undefined;
		for (const ps of providerStates) {
			const found = ps.providerConfigs.find((pc) => pc.id === view.configId);
			if (found) {
				selectedConfig = found;
				parentProviderState = ps;
				break;
			}
		}

		if (selectedConfig && parentProviderState) {
			return (
				<ProviderForm
					provider={parentProviderState.provider}
					providerLabel={parentProviderState.label}
					providerConfig={selectedConfig}
					hasManagedAPIKey={selectedConfig.has_api_key}
					isEnvPreset={parentProviderState.isEnvPreset}
					baseURL={readOptionalString(selectedConfig.base_url) ?? ""}
					providerConfigsUnavailable={providerConfigsUnavailable}
					isProviderMutationPending={isProviderMutationPending}
					onCreateProvider={onCreateProvider}
					onUpdateProvider={onUpdateProvider}
					onDeleteProvider={async (id) => {
						await onDeleteProvider(id);
						if (canGoBack) {
							navigate(-1);
						} else {
							setSearchParams(
								(prev) => {
									const next = new URLSearchParams(prev);
									next.delete("providerConfig");
									next.delete("newProvider");
									return next;
								},
								{ replace: true },
							);
						}
					}}
					onBack={clearProviderView}
				/>
			);
		}
	}

	// ── Add view ──────────────────────────────────────────────
	if (view.mode === "add") {
		const addProviderState = providerStates.find(
			(ps) => ps.provider === view.provider,
		);

		if (addProviderState) {
			return (
				<ProviderForm
					provider={addProviderState.provider}
					providerLabel={addProviderState.label}
					providerConfig={undefined}
					hasManagedAPIKey={false}
					isEnvPreset={addProviderState.isEnvPreset}
					baseURL=""
					providerConfigsUnavailable={providerConfigsUnavailable}
					isProviderMutationPending={isProviderMutationPending}
					onCreateProvider={onCreateProvider}
					onUpdateProvider={onUpdateProvider}
					onDeleteProvider={async (id) => {
						await onDeleteProvider(id);
						if (canGoBack) {
							navigate(-1);
						} else {
							setSearchParams(
								(prev) => {
									const next = new URLSearchParams(prev);
									next.delete("providerConfig");
									next.delete("newProvider");
									return next;
								},
								{ replace: true },
							);
						}
					}}
					onBack={clearProviderView}
				/>
			);
		}
	}

	// ── List view ─────────────────────────────────────────────

	if (providerStates.length === 0) {
		return (
			<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
				No provider types were returned by the backend.
			</div>
		);
	}

	return (
		<>
			{sectionLabel && (
				<SectionHeader
					label={sectionLabel}
					description={
						sectionDescription ?? "Configure AI providers to use with Agents."
					}
					badge={sectionBadge}
					action={
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button size="sm" className="gap-1.5" aria-label="Add provider">
									<PlusIcon className="h-4 w-4" />
									Add
									<ChevronDownIcon className="h-3.5 w-3.5 text-content-secondary" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								{providerStates.map((ps) => (
									<DropdownMenuItem
										key={ps.provider}
										onClick={() => {
											onSelectedProviderChange(ps.provider);
											setSearchParams(
												{ newProvider: ps.provider },
												{ state: { pushed: true } },
											);
										}}
										className="gap-2"
									>
										<ProviderIcon provider={ps.provider} className="h-5 w-5" />
										{ps.label}
									</DropdownMenuItem>
								))}
							</DropdownMenuContent>
						</DropdownMenu>
					}
				/>
			)}
			<div>
				{/* Configured rows — one per database config */}
				{providerStates.flatMap((providerState) => {
					const dbConfigs = getProviderConfigs(providerState).filter(
						isLocalDatabaseConfig,
					);
					return dbConfigs.map((config, idx) => {
						const displayLabel =
							readOptionalString(config.display_name) ??
							(dbConfigs.length === 1
								? providerState.label
								: `${providerState.label} ${idx + 1}`);
						const isActive = config.enabled && config.has_api_key;

						return (
							<button
								type="button"
								key={config.id}
								aria-label={displayLabel}
								onClick={() => {
									onSelectedProviderChange(providerState.provider);
									setSearchParams(
										{ providerConfig: config.id },
										{ state: { pushed: true } },
									);
								}}
								className={cn(
									"flex w-full cursor-pointer items-center gap-3.5 bg-transparent border-0 p-0 px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
									"border-0 border-t border-solid border-border/50 first:border-t-0",
								)}
							>
								<ProviderIcon
									provider={providerState.provider}
									className="h-8 w-8 shrink-0"
								/>
								<span
									className={cn(
										"min-w-0 flex-1 truncate text-[15px] font-medium text-left",
										isActive
											? "text-content-primary"
											: "text-content-secondary",
									)}
								>
									{displayLabel}
								</span>
								{isActive ? (
									<CheckCircleIcon className="h-4 w-4 shrink-0 text-content-success" />
								) : (
									<CircleIcon className="h-4 w-4 shrink-0 text-content-secondary opacity-40" />
								)}
								<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
							</button>
						);
					});
				})}

				{/* Placeholder rows — one per family with no database configs */}
				{providerStates
					.filter(
						(ps) =>
							getProviderConfigs(ps).filter(isLocalDatabaseConfig).length === 0,
					)
					.map((providerState) => (
						<button
							type="button"
							key={providerState.provider}
							aria-label={providerState.label}
							onClick={() => {
								onSelectedProviderChange(providerState.provider);
								setSearchParams(
									{ newProvider: providerState.provider },
									{ state: { pushed: true } },
								);
							}}
							className={cn(
								"flex w-full cursor-pointer items-center gap-3.5 bg-transparent border-0 p-0 px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
								"border-0 border-t border-solid border-border/50",
							)}
						>
							<ProviderIcon
								provider={providerState.provider}
								className="h-8 w-8 shrink-0"
							/>
							<span className="min-w-0 flex-1 truncate text-[15px] font-medium text-content-secondary text-left">
								{providerState.label}
							</span>
							{providerState.hasEffectiveAPIKey ? (
								<CheckCircleIcon className="h-4 w-4 shrink-0 text-content-success" />
							) : (
								<CircleIcon className="h-4 w-4 shrink-0 text-content-secondary opacity-40" />
							)}
							<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
						</button>
					))}
			</div>
		</>
	);
};
