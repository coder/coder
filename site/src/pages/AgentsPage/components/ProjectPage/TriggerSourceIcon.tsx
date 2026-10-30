import { cn } from "cn";
import { ClockIcon, WebhookIcon } from "lucide-react";
import type { ChatProjectTriggerSource } from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { triggerSources } from "./triggerCatalog";

type TriggerSourceIconProps = {
	readonly source: ChatProjectTriggerSource;
	readonly className?: string;
};

/** The logo of the service that calls a trigger, or a clock for schedules. Decorative. */
export const TriggerSourceIcon: React.FC<TriggerSourceIconProps> = ({
	source,
	className,
}) => {
	const iconURL = triggerSources[source].iconURL;
	if (!iconURL) {
		const Icon = source === "schedule" ? ClockIcon : WebhookIcon;
		return (
			<Icon
				aria-hidden
				className={cn("size-4 shrink-0 text-content-secondary", className)}
			/>
		);
	}
	return (
		<ExternalImage
			src={iconURL}
			alt=""
			aria-hidden
			className={cn("size-4 shrink-0 object-contain", className)}
		/>
	);
};
