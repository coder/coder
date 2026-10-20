import type { AlertColor } from "#/components/Alert/Alert";
import { AlertVariant, ProvisionerAlert } from "./ProvisionerAlert";

type ProvisionerStatusAlertProps = {
	matchingProvisioners: number | undefined;
	availableProvisioners: number | undefined;
	tags: Record<string, string>;
	variant?: AlertVariant;
};

export const ProvisionerStatusAlert: React.FC<ProvisionerStatusAlertProps> = ({
	matchingProvisioners,
	availableProvisioners,
	tags,
	variant = AlertVariant.Standalone,
}) => {
	let title: string | undefined;
	let detail: string;
	let severity: AlertColor;
	switch (true) {
		case matchingProvisioners === 0:
			title = "No provisioner can run this build";
			detail =
				"The build is queued, but no provisioner accepts these tags. Ask your administrator to add one.";
			severity = "warning";
			break;
		case availableProvisioners === 0:
			title = "Build delayed";
			detail =
				"Provisioners with these tags haven't responded. Ask your administrator if the build doesn't start.";
			severity = "warning";
			break;
		default:
			detail = "The build is queued and starts when a provisioner is free.";
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
