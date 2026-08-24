import type { FC } from "react";
import type {
	Organization,
	ShareableWorkspaceOwners,
	UpdateOrganizationRequest,
} from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { DeleteOrganizationSection } from "./DeleteOrganizationSection";
import { OrganizationInfoForm } from "./OrganizationInfoForm";
import { WorkspaceSharingSection } from "./WorkspaceSharingSection";

interface OrganizationSettingsPageViewProps {
	organization: Organization;
	error: unknown;
	onSubmit: (values: UpdateOrganizationRequest) => Promise<void>;
	onDeleteOrganization: () => void;
	workspaceSharingGloballyDisabled?: boolean;
	shareableWorkspaceOwners?: ShareableWorkspaceOwners;
	onChangeShareableOwners?: (value: ShareableWorkspaceOwners) => void;
	isTogglingWorkspaceSharing?: boolean;
}

export const OrganizationSettingsPageView: FC<
	OrganizationSettingsPageViewProps
> = ({
	organization,
	error,
	onSubmit,
	onDeleteOrganization,
	workspaceSharingGloballyDisabled,
	shareableWorkspaceOwners = "none",
	onChangeShareableOwners,
	isTogglingWorkspaceSharing = false,
}) => {
	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Settings</SettingsHeaderTitle>
			</SettingsHeader>

			<OrganizationInfoForm
				organization={organization}
				error={error}
				onSubmit={onSubmit}
			/>

			{onChangeShareableOwners && (
				<WorkspaceSharingSection
					organizationId={organization.id}
					workspaceSharingGloballyDisabled={workspaceSharingGloballyDisabled}
					shareableWorkspaceOwners={shareableWorkspaceOwners}
					onChangeShareableOwners={onChangeShareableOwners}
					isTogglingWorkspaceSharing={isTogglingWorkspaceSharing}
				/>
			)}

			{!organization.is_default && (
				<DeleteOrganizationSection
					organizationName={organization.name}
					onDeleteOrganization={onDeleteOrganization}
				/>
			)}
		</div>
	);
};
