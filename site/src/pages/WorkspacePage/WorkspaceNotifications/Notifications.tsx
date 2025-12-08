import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { AlertProps } from "components/Alert/Alert";
import { Button, type ButtonProps } from "components/Button/Button";
import { Pill } from "components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { type FC, type ReactNode, useState } from "react";
import type { ThemeRole } from "theme/roles";
import { cn } from "utils/cn";

export type NotificationItem = {
	title: string;
	severity: AlertProps["severity"];
	detail?: ReactNode;
	actions?: ReactNode;
};

type NotificationsProps = {
	items: NotificationItem[];
	severity: ThemeRole;
	icon: ReactNode;
};

export const Notifications: FC<NotificationsProps> = ({
	items,
	severity,
	icon,
}) => {
	const [isOpen, setIsOpen] = useState(false);
	const theme = useTheme();

	return (
		<TooltipProvider>
			<Tooltip open={isOpen} onOpenChange={setIsOpen} delayDuration={0}>
				<TooltipTrigger asChild>
					<div className="py-2" data-testid={`${severity}-notifications`}>
						<NotificationPill
							items={items}
							severity={severity}
							icon={icon}
							isTooltipOpen={isOpen}
						/>
					</div>
				</TooltipTrigger>
				<TooltipContent
					align="end"
					collisionPadding={16}
					className="max-w-[400px] p-0 bg-surface-secondary border-surface-quaternary text-sm  text-white"
					style={{
						borderColor: theme.roles[severity].outline,
					}}
				>
					{items.map((n) => (
						<NotificationItem notification={n} key={n.title} />
					))}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

type NotificationPillProps = NotificationsProps & {
	isTooltipOpen: boolean;
};

const NotificationPill: FC<NotificationPillProps> = ({
	items,
	severity,
	icon,
	isTooltipOpen,
}) => {
	return (
		<Pill
			icon={icon}
			css={(theme) => ({
				"& svg": { color: theme.roles[severity].outline },
				borderColor: isTooltipOpen ? theme.roles[severity].outline : undefined,
			})}
		>
			{items.length}
		</Pill>
	);
};

interface NotificationItemProps {
	notification: NotificationItem;
}

const NotificationItem: FC<NotificationItemProps> = ({ notification }) => {
	return (
		<article
			className={cn([
				"p-5 leading-normal border-0 border-t border-solid border-zinc-700 first:border-t-0",
			])}
		>
			<h4 className="m-0 font-medium">{notification.title}</h4>
			{notification.detail && (
				<p className="m-0 text-content-secondary leading-6 block mt-2">
					{notification.detail}
				</p>
			)}
			<div className="mt-2 flex items-center gap-1">{notification.actions}</div>
		</article>
	);
};

export const NotificationActionButton: FC<ButtonProps> = (props) => {
	return <Button variant="outline" size="sm" {...props} />;
};
