import { type FC, type ReactNode, useState } from "react";
import type { AlertProps } from "#/components/Alert/Alert";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { Pill } from "#/components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

export type NotificationItem = {
	title: string;
	severity: AlertProps["severity"];
	detail?: ReactNode;
	actions?: ReactNode;
};

type NotificationSeverity = "warning" | "info";

type NotificationsProps = {
	items: NotificationItem[];
	severity: NotificationSeverity;
	icon: ReactNode;
};

export const Notifications: FC<NotificationsProps> = ({
	items,
	severity,
	icon,
}) => {
	const [isOpen, setIsOpen] = useState(false);

	// Map severity to border color for the open popover.
	const outlineColors: Record<string, string> = {
		warning: "hsl(var(--border-warning))",
		info: "hsl(var(--border-default))",
	};

	return (
		<Popover open={isOpen} onOpenChange={setIsOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="py-2 bg-transparent border-none cursor-pointer"
					data-testid={`${severity}-notifications`}
				>
					<NotificationPill
						items={items}
						severity={severity}
						icon={icon}
						isOpen={isOpen}
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				collisionPadding={16}
				className="max-w-[400px] p-0 w-auto bg-surface-secondary border-surface-quaternary text-sm text-content-primary"
				style={{
					borderColor: outlineColors[severity],
				}}
			>
				{items.map((n) => (
					<NotificationItem notification={n} key={n.title} />
				))}
			</PopoverContent>
		</Popover>
	);
};

type NotificationPillProps = NotificationsProps & {
	isOpen: boolean;
};

const NotificationPill: FC<NotificationPillProps> = ({
	items,
	severity,
	icon,
	isOpen,
}) => {
	// Keep the pill background neutral; only color the icon and
	// the border when the popover is open.
	const iconColorClass =
		severity === "warning" ? "[&_svg]:text-content-warning" : "";

	return (
		<Pill
			icon={icon}
			className={cn(
				iconColorClass,
				isOpen && severity === "warning" && "border-amber-300",
				isOpen && severity === "info" && "border-zinc-400",
			)}
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
		<article className="p-5 leading-normal border-0 border-t border-solid first:border-t-0">
			<h4 className="m-0 font-medium">{notification.title}</h4>
			{notification.detail && (
				<p className="m-0 text-content-secondary leading-relaxed block mt-2">
					{notification.detail}
				</p>
			)}
			<div className="mt-2 flex items-center gap-1">{notification.actions}</div>
		</article>
	);
};

export const NotificationActionButton: FC<ButtonProps> = (props) => {
	return <Button variant="default" size="sm" {...props} />;
};
