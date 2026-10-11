import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	agentHoursGroupAllotments,
	deleteAgentHoursGroupAllotment,
	organizationAgentHoursUsage,
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
	// Groups deleted in another tab must leave the Add candidates.
	const groupsQuery = useQuery({
		...groupsByOrganization(organization.name),
		refetchOnWindowFocus: true,
	});
	const usageQuery = useQuery(organizationAgentHoursUsage(organization.id));
	const upsertMutation = useMutation(
		upsertAgentHoursGroupAllotment(queryClient),
	);
	const deleteMutation = useMutation(
		deleteAgentHoursGroupAllotment(queryClient),
	);

	return (
		<OrganizationAgentHoursView
			organization={organization}
			licenseHours={licenseHours}
			groupAllotments={groupAllotmentsQuery.data}
			groups={groupsQuery.data}
			error={groupAllotmentsQuery.error ?? groupsQuery.error}
			usage={usageQuery.data}
			usageError={usageQuery.error}
			onSave={(groupId, allotmentBps) =>
				upsertMutation.mutateAsync({ groupId, allotmentBps })
			}
			onRemove={deleteMutation.mutateAsync}
		/>
	);
};
