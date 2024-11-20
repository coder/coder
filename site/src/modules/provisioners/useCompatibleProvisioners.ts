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
