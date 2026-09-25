import { BellIcon, BellOffIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { useWebpushNotifications } from "#/contexts/useWebpushNotifications";

type WebPushButtonProps = {
	webPush: ReturnType<typeof useWebpushNotifications>;
	onToggle: () => Promise<void> | void;
};

export const WebPushButton: FC<WebPushButtonProps> = ({
	webPush,
	onToggle,
}) => {
	if (!webPush.enabled) {
		return null;
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					disabled={webPush.loading}
					onClick={onToggle}
					aria-label={
						webPush.subscribed
							? "Disable notifications"
							: "Enable notifications"
					}
					className="size-7 text-content-secondary hover:text-content-primary"
				>
					{webPush.loading ? (
						<Spinner size="sm" loading />
					) : webPush.subscribed ? (
						<BellIcon className="text-content-success" />
					) : (
						<BellOffIcon className="text-content-secondary" />
					)}
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				{webPush.subscribed ? "Disable notifications" : "Enable notifications"}
			</TooltipContent>
		</Tooltip>
	);
};
