import type { InboxNotification } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { SquareCheckBig } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { relativeTime } from "utils/time";

type InboxItemProps = {
	notification: InboxNotification;
	onMarkNotificationAsRead: (notificationId: string) => void;
};

export const InboxItem: FC<InboxItemProps> = ({
	notification,
	onMarkNotificationAsRead,
}) => {
	return (
		<div
			className="flex items-stretch gap-3 p-3 group"
			role="menuitem"
			tabIndex={-1}
		>
			<div className="flex-shrink-0">
				<Avatar fallback="AR" />
			</div>

			<div className="flex flex-col gap-3 flex-1">
				<span className="text-content-secondary text-sm font-medium">
					{notification.content}
				</span>
				<div className="flex items-center gap-1">
					{notification.actions.map((action) => {
						return (
							<Button variant="outline" size="sm" key={action.label} asChild>
								<RouterLink to={action.url}>{action.label}</RouterLink>
							</Button>
						);
					})}
				</div>
			</div>

			<div className="w-12 flex flex-col items-end flex-shrink-0">
				{notification.read_at === null && (
					<>
						<div className="group-focus:hidden group-hover:hidden size-2.5 rounded-full bg-highlight-sky">
							<span className="sr-only">Unread</span>
						</div>

						<Button
							onClick={() => onMarkNotificationAsRead(notification.id)}
							className="hidden group-focus:flex group-hover:flex bg-surface-primary"
							variant="outline"
							size="sm"
						>
							<SquareCheckBig />
							mark as read
						</Button>
					</>
				)}

				<span className="mt-auto text-content-secondary text-xs font-medium whitespace-nowrap">
					{relativeTime(new Date(notification.created_at))}
				</span>
			</div>
		</div>
	);
};
