import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	agentHoursGroupAllotments,
	deleteAgentHoursGroupAllotment,
	upsertAgentHoursGroupAllotment,
} from "#/api/queries/agentHours";
import { groupsByOrganization } from "#/api/queries/groups";
import type { Organization } from "#/api/typesGenerated";
import { OrganizationAgentHoursView } from "./OrganizationAgentHoursView";

type OrganizationAgentHoursProps = {
	organization: Organization;
	licenseHours: number | undefined;
};

export const OrganizationAgentHours: React.FC<OrganizationAgentHoursProps> = ({
	organization,
	licenseHours,
}) => {
	const queryClient = useQueryClient();
	const groupAllotmentsQuery = useQuery(
		agentHoursGroupAllotments(organization.id),
	);
	const groupsQuery = useQuery(groupsByOrganization(organization.name));
	const upsertMutation = useMutation(
		upsertAgentHoursGroupAllotment(queryClient, organization.id),
	);
	const deleteMutation = useMutation(
		deleteAgentHoursGroupAllotment(queryClient, organization.id),
	);

	return (
		<OrganizationAgentHoursView
			licenseHours={licenseHours}
			groupAllotments={groupAllotmentsQuery.data}
			groups={groupsQuery.data}
			error={groupAllotmentsQuery.error ?? groupsQuery.error}
			onSave={(groupId, allotmentBps) =>
				upsertMutation.mutateAsync({ groupId, allotmentBps })
			}
			onRemove={deleteMutation.mutateAsync}
		/>
	);
};
