import { type FC, type ReactNode, useState } from "react";
import type { AlertProps } from "#/components/Alert/Alert";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { Pill } from "#/components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import type { ThemeRole } from "#/theme/roles";
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

// Maps a ThemeRole severity to Tailwind classes for the role's outline
// color. These are the closest semantic matches available in the design
// token system.
const severityStyles: Record<ThemeRole, { svgColor: string; border: string }> =
	{
		error: {
			svgColor: "[&_svg]:text-border-destructive",
			border: "border-border-destructive",
		},
		warning: {
			svgColor: "[&_svg]:text-border-warning",
			border: "border-border-warning",
		},
		notice: {
			svgColor: "[&_svg]:text-border-pending",
			border: "border-border-pending",
		},
		info: {
			svgColor: "[&_svg]:text-content-secondary",
			border: "border-border",
		},
		success: {
			svgColor: "[&_svg]:text-border-success",
			border: "border-border-success",
		},
		active: {
			svgColor: "[&_svg]:text-border-pending",
			border: "border-border-pending",
		},
		inactive: {
			svgColor: "[&_svg]:text-content-disabled",
			border: "border-border",
		},
		danger: {
			svgColor: "[&_svg]:text-border-warning",
			border: "border-border-warning",
		},
		preview: {
			svgColor: "[&_svg]:text-border-purple",
			border: "border-border-purple",
		},
	};

export const Notifications: FC<NotificationsProps> = ({
	items,
	severity,
	icon,
}) => {
	const [isOpen, setIsOpen] = useState(false);

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
				className={cn(
					"max-w-[400px] p-0 w-auto bg-surface-secondary text-sm text-content-primary",
					severityStyles[severity].border,
				)}
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
	return (
		<Pill
			icon={icon}
			className={cn(
				severityStyles[severity].svgColor,
				isOpen && severityStyles[severity].border,
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
