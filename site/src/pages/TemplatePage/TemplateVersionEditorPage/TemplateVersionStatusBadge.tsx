import { CheckIcon, CircleAlertIcon, HourglassIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { TemplateVersion } from "#/api/typesGenerated";
import { Badge, type BadgeProps } from "#/components/Badge/Badge";
import { Spinner } from "#/components/Spinner/Spinner";
import { getPendingStatusLabel } from "#/utils/provisionerJob";

interface TemplateVersionStatusBadgeProps {
	version: TemplateVersion;
}

export const TemplateVersionStatusBadge: FC<
	TemplateVersionStatusBadgeProps
> = ({ version }) => {
	const { text, icon, variant } = getStatus(version);
	return (
		<Badge variant={variant} title={`Build status is ${text}`} role="status">
			{icon}
			{text}
		</Badge>
	);
};

const getStatus = (
	version: TemplateVersion,
): {
	variant: NonNullable<BadgeProps["variant"]>;
	text: string;
	icon: ReactNode;
} => {
	switch (version.job.status) {
		case "running":
			return {
				variant: "info",
				text: "Running",
				icon: <Spinner loading />,
			};
		case "pending":
			return {
				variant: "info",
				text: getPendingStatusLabel(version.job),
				icon: <HourglassIcon className="size-icon-sm" />,
			};
		case "canceling":
			return {
				variant: "default",
				text: "Canceling",
				icon: <Spinner loading />,
			};
		case "canceled":
			return {
				variant: "default",
				text: "Canceled",
				icon: <CircleAlertIcon className="size-icon-sm" />,
			};
		case "unknown":
		case "failed":
			return {
				variant: "destructive",
				text: "Failed",
				icon: <CircleAlertIcon className="size-icon-sm" />,
			};
		case "succeeded":
			return {
				variant: "green",
				text: "Success",
				icon: <CheckIcon className="size-icon-sm" />,
			};
	}
};
