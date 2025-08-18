import type { TemplateVersion } from "api/typesGenerated";
import { Pill, PillSpinner } from "components/Pill/Pill";
import { CheckIcon, CircleAlertIcon, HourglassIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { ThemeRole } from "theme/roles";
import { getPendingStatusLabel } from "utils/provisionerJob";

interface TemplateVersionStatusBadgeProps {
	version: TemplateVersion;
}

export const TemplateVersionStatusBadge: FC<
	TemplateVersionStatusBadgeProps
> = ({ version }) => {
	const { text, icon, type } = getStatus(version);
	return (
		<Pill
			icon={icon}
			type={type}
			title={`Build status is ${text}`}
			role="status"
		>
			{text}
		</Pill>
	);
};

const getStatus = (
	version: TemplateVersion,
): {
	type?: ThemeRole;
	text: string;
	icon: ReactNode;
} => {
	switch (version.job.status) {
		case "running":
			return {
				type: "active",
				text: "Running",
				icon: <PillSpinner />,
			};
		case "pending":
			return {
				type: "active",
				text: getPendingStatusLabel(version.job),
				icon: <HourglassIcon className="size-icon-sm" />,
			};
		case "canceling":
			return {
				type: "inactive",
				text: "Canceling",
				icon: <PillSpinner />,
			};
		case "canceled":
			return {
				type: "inactive",
				text: "Canceled",
				icon: <CircleAlertIcon className="size-icon-sm" />,
			};
		case "unknown":
		case "failed":
			return {
				type: "error",
				text: "Failed",
				icon: <CircleAlertIcon className="size-icon-sm" />,
			};
		case "succeeded":
			return {
				type: "success",
				text: "Success",
				icon: <CheckIcon className="size-icon-sm" />,
			};
	}
};
