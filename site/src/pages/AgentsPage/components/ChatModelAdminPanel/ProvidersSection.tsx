import { CheckCircleIcon, ChevronRightIcon, CircleIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { SectionHeader } from "../SectionHeader";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ProviderForm } from "./ProviderForm";
import { ProviderIcon } from "./ProviderIcon";

type ProviderView = { mode: "list" } | { mode: "detail"; provider: string };

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
		const providerParam = searchParams.get("provider");
		if (providerParam) {
			const exists = providerStates.some((ps) => ps.provider === providerParam);
			return exists
				? { mode: "detail", provider: providerParam }
				: { mode: "list" };
		}
		return { mode: "list" };
	})();

	// Clear provider search param and return to the list.
	const clearProviderView = () => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.delete("provider");
			return next;
		});
	};

	// ── Detail view ───────────────────────────────────────────
	const detailProvider =
		view.mode === "detail"
			? providerStates.find((ps) => ps.provider === view.provider)
			: undefined;

	if (view.mode === "detail" && detailProvider) {
		return (
			<ProviderForm
				providerState={detailProvider}
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
								next.delete("provider");
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
				/>
			)}
			<div>
				{providerStates.map((providerState, i) => (
					<button
						type="button"
						key={providerState.provider}
						aria-label={providerState.label}
						onClick={() => {
							onSelectedProviderChange(providerState.provider);
							setSearchParams(
								{ provider: providerState.provider },
								{ state: { pushed: true } },
							);
						}}
						className={cn(
							"flex w-full cursor-pointer items-center gap-3.5 bg-transparent border-0 p-0 px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
							i > 0 && "border-0 border-t border-solid border-border/50",
						)}
					>
						<ProviderIcon
							provider={providerState.provider}
							className="h-8 w-8 shrink-0"
						/>
						<span className="min-w-0 flex-1 truncate text-[15px] font-medium text-content-primary text-left">
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
