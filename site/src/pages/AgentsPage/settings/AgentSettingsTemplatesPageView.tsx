import { type FC, type FormEvent, useState } from "react";
import type { UseMutationResult, UseQueryResult } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MultiSelectCombobox,
	type Option,
} from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "#/components/Spinner/Spinner";
import { AdminBadge } from "../components/AdminBadge";
import { SectionHeader } from "../components/SectionHeader";

interface AgentSettingsTemplatesPageViewProps {
	templatesQuery: UseQueryResult<TypesGen.Template[]>;
	allowlistQuery: UseQueryResult<TypesGen.ChatTemplateAllowlist>;
	saveAllowlistMutation: UseMutationResult<
		void,
		unknown,
		TypesGen.ChatTemplateAllowlist
	>;
}

export const AgentSettingsTemplatesPageView: FC<
	AgentSettingsTemplatesPageViewProps
> = ({ templatesQuery, allowlistQuery, saveAllowlistMutation }) => {
	const [localSelection, setLocalSelection] = useState<Option[] | null>(null);

	// Map all templates to MultiSelectCombobox options.
	const allOptions: Option[] = (templatesQuery.data ?? []).map((t) => ({
		value: t.id,
		label: t.display_name || t.name,
		icon: t.icon,
	}));

	// Build a lookup from template ID to Option for resolving server IDs.
	const optionsByID = new Map(allOptions.map((o) => [o.value, o]));

	// Resolve the server-side allowlist IDs into Option objects.
	const serverSelection: Option[] = (allowlistQuery.data?.template_ids ?? [])
		.map((id) => optionsByID.get(id))
		.filter((o) => o !== undefined);

	const currentSelection = localSelection ?? serverSelection;

	const serverSet = new Set(serverSelection.map((o) => o.value));
	const isDirty =
		localSelection !== null &&
		(localSelection.length !== serverSet.size ||
			localSelection.some((o) => !serverSet.has(o.value)));

	const isSaving = saveAllowlistMutation.isPending;

	const handleSave = (event: FormEvent) => {
		event.preventDefault();
		if (!isDirty) return;
		saveAllowlistMutation.mutate(
			{ template_ids: currentSelection.map((o) => o.value) },
			{ onSuccess: () => setLocalSelection(null) },
		);
	};

	const isLoading = templatesQuery.isLoading || allowlistQuery.isLoading;

	return (
		<div className="space-y-6">
			<SectionHeader
				label="Templates"
				description="Restrict which templates agents can use to create workspaces. When no templates are selected, all templates are available."
				badge={<AdminBadge />}
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

			{!isLoading && (templatesQuery.error || allowlistQuery.error) && (
				<div className="flex min-h-[120px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						Failed to load template data.
					</p>
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => {
							void templatesQuery.refetch();
							void allowlistQuery.refetch();
						}}
					>
						Retry
					</Button>
				</div>
			)}

			{!isLoading && !templatesQuery.error && !allowlistQuery.error && (
				<form
					className="space-y-3"
					onSubmit={(event) => void handleSave(event)}
				>
					<MultiSelectCombobox
						key={serverSelection.map((o) => o.value).join(",")}
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

					{saveAllowlistMutation.isError && (
						<p role="alert" className="m-0 text-xs text-content-destructive">
							Failed to save template allowlist.
						</p>
					)}
				</form>
			)}
		</div>
	);
};
