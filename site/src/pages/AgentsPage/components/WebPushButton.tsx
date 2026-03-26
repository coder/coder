import { useWebpushNotifications } from "contexts/useWebpushNotifications";
import { BellIcon, BellOffIcon } from "lucide-react";
import type { FC } from "react";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export const WebPushButton: FC = () => {
	const webPush = useWebpushNotifications();

	if (!webPush.enabled) {
		return null;
	}

	const handleClick = async () => {
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
			} else {
				await webPush.subscribe();
			}
		} catch (error) {
			const action = webPush.subscribed ? "disable" : "enable";
			toast.error(getErrorMessage(error, `Failed to ${action} notifications.`));
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
					aria-label={
						webPush.subscribed
							? "Disable notifications"
							: "Enable notifications"
					}
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
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
