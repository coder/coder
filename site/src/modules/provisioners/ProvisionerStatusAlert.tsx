import type { AlertColor } from "components/Alert/Alert";
import type { FC } from "react";
import { AlertVariant, ProvisionerAlert } from "./ProvisionerAlert";

interface ProvisionerStatusAlertProps {
	matchingProvisioners: number | undefined;
	availableProvisioners: number | undefined;
	tags: Record<string, string>;
	variant?: AlertVariant;
}

export const ProvisionerStatusAlert: FC<ProvisionerStatusAlertProps> = ({
	matchingProvisioners,
	availableProvisioners,
	tags,
	variant = AlertVariant.Standalone,
}) => {
	let title: string;
	let detail: string;
	let severity: AlertColor;
	switch (true) {
		case matchingProvisioners === 0:
			title = "Build pending provisioner deployment";
			detail =
				"Your build has been enqueued, but there are no provisioners that accept the required tags. Once a compatible provisioner becomes available, your build will continue. Please contact your administrator.";
			severity = "warning";
			break;
		case availableProvisioners === 0:
			title = "Build delayed";
			detail =
				"Provisioners that accept the required tags have not responded for longer than expected. This may delay your build. Please contact your administrator if your build does not complete.";
			severity = "warning";
			break;
		default:
			title = "Build enqueued";
			detail =
				"Your build has been enqueued and will begin once a provisioner becomes available to process it.";
			severity = "info";
	}

	return (
		<ProvisionerAlert
			title={title}
			detail={detail}
			severity={severity}
			tags={tags}
			variant={variant}
		/>
	);
};
