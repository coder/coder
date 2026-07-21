import type { FC } from "react";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import { CompactOrgSelector } from "#/pages/AgentsPage/components/ChatElements";

export const AIResourceOrganizationSelector: FC = () => {
	const { organization, organizations, selectOrganization } =
		useAIResourceOrganization();
	if (organizations.length < 2) {
		return null;
	}
	return (
		<CompactOrgSelector
			value={organization ?? null}
			options={organizations}
			onChange={selectOrganization}
		/>
	);
};
