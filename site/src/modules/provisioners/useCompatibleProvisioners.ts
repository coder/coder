import { ProvisionerDaemon } from "api/typesGenerated";

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
