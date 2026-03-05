import { getErrorMessage } from "api/errors";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useWebpushNotifications } from "contexts/useWebpushNotifications";
import { BellIcon, BellOffIcon, Loader2Icon } from "lucide-react";
import type { FC } from "react";
import { toast } from "sonner";

export const WebPushButton: FC = () => {
	const webPush = useWebpushNotifications();

	if (!webPush.enabled) {
		return null;
	}

	const handleClick = async () => {
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
				toast.success("Notifications disabled.");
			} else {
				await webPush.subscribe();
				toast.success("Notifications enabled.");
			}
		} catch (error) {
			if (webPush.subscribed) {
				toast.error(getErrorMessage(error, "Failed to disable notifications."));
			} else {
				toast.error(getErrorMessage(error, "Failed to enable notifications."));
			}
		}
	};

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					disabled={webPush.loading}
					onClick={handleClick}
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
				>
					{webPush.loading ? (
						<Loader2Icon className="animate-spin" />
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
