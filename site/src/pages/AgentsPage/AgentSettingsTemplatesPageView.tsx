import { type FC, type FormEvent, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MultiSelectCombobox,
	type Option,
} from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "#/components/Spinner/Spinner";
import { SectionHeader } from "./components/SectionHeader";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AgentSettingsTemplatesPageViewProps {
	// Raw query data
	templatesData: TypesGen.Template[] | undefined;
	allowlistData: TypesGen.ChatTemplateAllowlist | undefined;
	isLoading: boolean;
	hasError: boolean;
	onRetry: () => void;

	// Mutation
	onSaveAllowlist: (
		req: TypesGen.ChatTemplateAllowlist,
		options?: MutationCallbacks,
	) => void;
	isSaving: boolean;
	isSaveError: boolean;
}

export const AgentSettingsTemplatesPageView: FC<
	AgentSettingsTemplatesPageViewProps
> = ({
	templatesData,
	allowlistData,
	isLoading,
	hasError,
	onRetry,
	onSaveAllowlist,
	isSaving,
	isSaveError,
}) => {
	// ── Local form state ──
	const [localSelection, setLocalSelection] = useState<Option[] | null>(null);

	// ── Derived state ──
	const allOptions: Option[] = (templatesData ?? []).map((t) => ({
		value: t.id,
		label: t.display_name || t.name,
		icon: t.icon,
	}));

	const optionsByID = new Map(allOptions.map((o) => [o.value, o]));

	const serverSelection: Option[] = (allowlistData?.template_ids ?? [])
		.map((id) => optionsByID.get(id))
		.filter((o) => o !== undefined);

	const currentSelection = localSelection ?? serverSelection;

	const serverSet = new Set(serverSelection.map((o) => o.value));
	const isDirty =
		localSelection !== null &&
		(localSelection.length !== serverSet.size ||
			localSelection.some((o) => !serverSet.has(o.value)));

	const serverSelectionKey = serverSelection.map((o) => o.value).join(",");

	// ── Event handlers ──
	const handleSave = (event: FormEvent) => {
		event.preventDefault();
		if (!isDirty) return;
		onSaveAllowlist(
			{ template_ids: currentSelection.map((o) => o.value) },
			{ onSuccess: () => setLocalSelection(null) },
		);
	};

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Templates"
				description="Restrict which templates agents can use to create workspaces. When no templates are selected, all templates are available."
			/>

			{isLoading && (
				<div
					role="status"
					aria-label="Loading templates"
					className="flex min-h-[120px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}

			{!isLoading && hasError && (
				<div className="flex min-h-[120px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						Failed to load template data.
					</p>
					<Button variant="outline" size="sm" type="button" onClick={onRetry}>
						Retry
					</Button>
				</div>
			)}

			{!isLoading && !hasError && (
				<form
					className="space-y-3"
					onSubmit={(event) => void handleSave(event)}
				>
					<MultiSelectCombobox
						key={serverSelectionKey}
						inputProps={{ "aria-label": "Select allowed templates" }}
						options={allOptions}
						defaultOptions={currentSelection}
						value={currentSelection}
						onChange={setLocalSelection}
						placeholder="Select templates..."
						emptyIndicator={
							<p className="text-center text-sm text-content-secondary">
								No templates found.
							</p>
						}
						disabled={isSaving}
						hidePlaceholderWhenSelected
						data-testid="template-allowlist-select"
					/>
					<p
						aria-live="polite"
						role="status"
						className="m-0 text-xs text-content-secondary"
					>
						{currentSelection.length > 0
							? `${currentSelection.length} template${currentSelection.length !== 1 ? "s" : ""} selected`
							: "No templates selected \u2014 all templates are available"}
					</p>

					<div className="flex justify-end">
						<Button size="sm" type="submit" disabled={isSaving || !isDirty}>
							Save
						</Button>
					</div>

					{isSaveError && (
						<p role="alert" className="m-0 text-xs text-content-destructive">
							Failed to save template allowlist.
						</p>
					)}
				</form>
			)}
		</div>
	);
};
