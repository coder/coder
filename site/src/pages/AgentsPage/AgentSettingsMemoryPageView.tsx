import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Switch } from "#/components/Switch/Switch";
import { MemorySection } from "./components/MemorySection";
import { SectionHeader } from "./components/SectionHeader";

interface AgentSettingsMemoryPageViewProps {
	organizations: readonly TypesGen.Organization[];
	selectedOrganization: TypesGen.Organization | undefined;
	settings: TypesGen.ChatPersonalMemorySettings | undefined;
	settingsError: unknown;
	isLoadingSettings: boolean;
	isSavingSettings: boolean;
	isSaveSettingsError: boolean;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	onSaveSettings: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatPersonalMemorySettingsRequest,
		unknown
	>;
}

export const AgentSettingsMemoryPageView: FC<
	AgentSettingsMemoryPageViewProps
> = ({
	organizations,
	selectedOrganization,
	settings,
	settingsError,
	isLoadingSettings,
	isSavingSettings,
	isSaveSettingsError,
	onSelectOrganization,
	onSaveSettings,
}) => {
	const enabled = settings?.enabled ?? false;
	const organizationId = selectedOrganization?.id ?? "";

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Memory"
				description="Facts the agent saves from your chats that are not in a project. Every such chat you own in this organization can read them."
			/>
			<div className="flex items-center justify-between gap-4">
				<div>
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Save and use personal memory
					</h3>
				</div>
				<Switch
					checked={enabled}
					onCheckedChange={(nextEnabled) =>
						onSaveSettings({ enabled: nextEnabled })
					}
					aria-label="Save and use personal memory"
					disabled={
						isLoadingSettings || isSavingSettings || Boolean(settingsError)
					}
				/>
			</div>
			{isSaveSettingsError && (
				<p className="m-0 text-sm text-content-destructive">
					Failed to save your personal memory preference.
				</p>
			)}
			{settingsError ? <ErrorAlert error={settingsError} /> : null}
			{organizations.length > 1 && selectedOrganization && (
				<OrganizationAutocomplete
					value={selectedOrganization}
					options={organizations}
					ariaLabel={`Organization ${getOrganizationLabel(selectedOrganization, organizations)}`}
					triggerClassName="w-60"
					optionsTabbable
					onChange={(organization) => {
						if (organization) onSelectOrganization(organization);
					}}
				/>
			)}
			{!enabled && !isLoadingSettings && !settingsError && (
				<p className="m-0 text-sm text-content-secondary">
					New chats will not read or write personal memory while this setting is
					off.
				</p>
			)}
			{organizationId && (
				<MemorySection
					scope={{ kind: "personal", organizationId }}
					title="Saved memories"
					description=""
				/>
			)}
		</div>
	);
};
