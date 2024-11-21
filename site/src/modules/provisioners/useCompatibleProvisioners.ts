import { API } from "api/api";
import { ProvisionerDaemon } from "api/typesGenerated";
import { useEffect, useState } from "react";

export const useCompatibleProvisioners = (organization: string | undefined, tags: Record<string, string> | undefined) => {
	const [compatibleProvisioners, setCompatibleProvisioners] = useState<ProvisionerDaemon[]>([])

	useEffect(() => {
		(async () => {
			if (!organization) {
				setCompatibleProvisioners([])
				return
			}

			try {
				const provisioners = await API.getProvisionerDaemonsByOrganization(
					organization,
					tags,
				);

				setCompatibleProvisioners(provisioners);
			} catch (error) {
				setCompatibleProvisioners([])
			}
		})();
	}, [organization, tags])

	return compatibleProvisioners
}

export const provisionersUnhealthy = (provisioners : ProvisionerDaemon[]) => {
	return provisioners.reduce((allUnhealthy, provisioner) => {
		if (!allUnhealthy) {
			// If we've found one healthy provisioner, then we don't need to look at the rest
			return allUnhealthy;
		}
		// Otherwise, all provisioners so far have been unhealthy, so we check the next one

		// If a provisioner has no last_seen_at value, then it's considered unhealthy
		if (!provisioner.last_seen_at) {
			return allUnhealthy;
		}

		// If a provisioner has not been seen within the last 60 seconds, then it's considered unhealthy
		const lastSeen = new Date(provisioner.last_seen_at);
		const oneMinuteAgo = new Date(Date.now() - 60000);
		const unhealthy = lastSeen < oneMinuteAgo;


		return allUnhealthy && unhealthy;
	}, true);
}
