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
					<div
						css={styles.pillContainer}
						data-testid={`${severity}-notifications`}
					>
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
		<article css={styles.notificationItem}>
			<h4 css={{ margin: 0, fontWeight: 500 }}>{notification.title}</h4>
			{notification.detail && (
				<p css={styles.notificationDetail}>{notification.detail}</p>
			)}
			<div className="mt-2 flex items-center gap-1">{notification.actions}</div>
		</article>
	);
};

export const NotificationActionButton: FC<ButtonProps> = (props) => {
	return <Button variant="outline" size="sm" {...props} />;
};

const styles = {
	// Adds some spacing from the Tooltip content
	pillContainer: {
		padding: "8px 0",
	},
	notificationItem: (theme) => ({
		padding: 20,
		lineHeight: "1.5",
		borderTop: `1px solid ${theme.palette.divider}`,

		"&:first-of-type": {
			borderTop: 0,
		},
	}),
	notificationDetail: (theme) => ({
		margin: 0,
		color: theme.palette.text.secondary,
		lineHeight: 1.6,
		display: "block",
		marginTop: 8,
	}),
} satisfies Record<string, Interpolation<Theme>>;
