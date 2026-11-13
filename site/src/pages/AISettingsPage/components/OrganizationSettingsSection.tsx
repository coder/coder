import type { Organization } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { SettingsSection } from "./SettingsSection";

type OrganizationSettingsSectionProps = {
	title: string;
	description: string;
	organization: Organization;
	organizations: readonly Organization[];
	onSelectOrganization: (organization: Organization) => void;
	requestedOrganizationDenied: boolean;
	children: React.ReactNode;
};

/**
 * A settings section for one organization, with a picker when the user can
 * choose between several organizations.
 */
export const OrganizationSettingsSection: React.FC<
	OrganizationSettingsSectionProps
> = ({
	title,
	description,
	organization,
	organizations,
	onSelectOrganization,
	requestedOrganizationDenied,
	children,
}) => (
	<SettingsSection
		title={title}
		description={description}
		actions={
			organizations.length > 1 && (
				<OrganizationAutocomplete
					value={organization}
					ariaLabel={`Organization ${getOrganizationLabel(
						organization,
						organizations,
					)}`}
					options={organizations}
					triggerClassName="w-60"
					optionsTabbable
					onChange={(nextOrganization) => {
						if (nextOrganization) {
							onSelectOrganization(nextOrganization);
						}
					}}
				/>
			)
		}
	>
		{requestedOrganizationDenied && (
			<Alert severity="warning">
				You don't have access to that organization, or it doesn't exist. Showing{" "}
				{organization.display_name || organization.name} instead.
			</Alert>
		)}
		{children}
	</SettingsSection>
);
