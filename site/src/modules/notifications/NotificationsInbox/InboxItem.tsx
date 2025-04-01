import type { InboxNotification } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import { SquareCheckBig } from "lucide-react";
import type { FC } from "react";
import Markdown from "react-markdown";
import { Link as RouterLink } from "react-router-dom";
import { relativeTime } from "utils/time";
import { InboxAvatar } from "./InboxAvatar";

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
				<InboxAvatar icon={notification.icon} />
			</div>

			<div className="flex flex-col gap-3 flex-1">
				<Markdown
					className="text-content-secondary prose-sm font-medium [overflow-wrap:anywhere]"
					components={{
						a: ({ node, ...props }) => {
							return <Link {...props} />;
						},
					}}
				>
					{notification.content}
				</Markdown>
				<div className="flex items-center gap-1">
					{notification.actions.map((action) => {
						return (
							<Button variant="outline" size="sm" key={action.label} asChild>
								<RouterLink
									to={action.url}
									onClick={() => {
										onMarkNotificationAsRead(notification.id);
									}}
								>
									{action.label}
								</RouterLink>
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
