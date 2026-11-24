import { SquareCheckBigIcon } from "lucide-react";
import { useId } from "react";
import Markdown from "react-markdown";
import { Link as RouterLink } from "react-router";
import type { InboxNotification } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { relativeTime } from "#/utils/time";
import { parseURL } from "#/utils/uri";
import { InboxAvatar } from "./InboxAvatar";

type InboxItemProps = {
	notification: InboxNotification;
	onMarkNotificationAsRead: (notificationId: string) => void;
};

export const InboxItem: React.FC<InboxItemProps> = ({
	notification,
	onMarkNotificationAsRead,
}) => {
	const contentId = useId();

	return (
		<div
			className="flex items-stretch gap-3 p-3 group"
			role="listitem"
			tabIndex={-1}
		>
			<div className="shrink-0">
				<InboxAvatar icon={notification.icon} />
			</div>

			<div className="flex flex-col gap-3 flex-1">
				<div
					id={contentId}
					className="text-content-secondary prose-sm font-medium wrap-anywhere"
				>
					<Markdown
						components={{
							a: ({ node, ...props }) => {
								// Notifications also link back into this deployment via
								// absolute URLs; only other origins open a new tab.
								const isExternal =
									props.href?.startsWith("http") &&
									parseURL(props.href)?.origin !== location.origin;
								return (
									<Link
										{...props}
										target={isExternal ? "_blank" : undefined}
										rel={isExternal ? "noreferrer" : undefined}
									/>
								);
							},
						}}
					>
						{notification.content}
					</Markdown>
				</div>
				<div className="flex items-center gap-1">
					{notification.actions.map((action) => {
						return (
							<Button variant="outline" size="sm" key={action.label} asChild>
								<RouterLink
									to={action.url}
									// Action labels like "View workspace" are ambiguous across
									// notifications without the message they act on.
									aria-describedby={contentId}
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

			<div className="w-12 flex flex-col items-end shrink-0">
				{notification.read_at === null && (
					<>
						{/* Outside the dot so it is still announced while the dot is hidden. */}
						<span className="sr-only">Unread</span>
						<div className="group-focus-within:hidden group-hover:hidden size-2.5 rounded-full bg-highlight-sky" />

						{/* Visually hidden instead of display: none so it stays in the tab order. */}
						<Button
							onClick={() => onMarkNotificationAsRead(notification.id)}
							aria-describedby={contentId}
							className="not-group-hover:not-group-focus-within:sr-only bg-surface-primary"
							variant="outline"
							size="sm"
						>
							<SquareCheckBigIcon />
							Mark as read
						</Button>
					</>
				)}

				<span className="mt-auto text-content-secondary text-xs whitespace-nowrap">
					{relativeTime(new Date(notification.created_at))}
				</span>
			</div>
		</div>
	);
};
