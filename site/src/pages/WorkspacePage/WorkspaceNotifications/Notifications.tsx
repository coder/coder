import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Button, { type ButtonProps } from "@mui/material/Button";
import type { AlertProps } from "components/Alert/Alert";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
	usePopover,
} from "components/deprecated/Popover/Popover";
import type { FC, ReactNode } from "react";
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
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<div
					css={styles.pillContainer}
					data-testid={`${severity}-notifications`}
				>
					<NotificationPill items={items} severity={severity} icon={icon} />
				</div>
			</PopoverTrigger>
			<PopoverContent
				horizontal="right"
				css={{
					"& .MuiPaper-root": {
						borderColor: theme.roles[severity].outline,
						maxWidth: 400,
					},
				}}
			>
				{items.map((n) => (
					<NotificationItem notification={n} key={n.title} />
				))}
			</PopoverContent>
		</Popover>
	);
};

const NotificationPill: FC<NotificationsProps> = ({
	items,
	severity,
	icon,
}) => {
	const popover = usePopover();

	return (
		<Pill
			icon={icon}
			css={(theme) => ({
				"& svg": { color: theme.roles[severity].outline },
				borderColor: popover.open ? theme.roles[severity].outline : undefined,
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
			<div css={{ marginTop: 8 }}>{notification.actions}</div>
		</article>
	);
};

export const NotificationActionButton: FC<ButtonProps> = (props) => {
	return (
		<Button
			variant="text"
			css={{
				textDecoration: "underline",
				paddingLeft: 0,
				paddingRight: 8,
				paddingTop: 0,
				paddingBottom: 0,
				height: "auto",
				minWidth: "auto",
				"&:hover": { background: "none", textDecoration: "underline" },
			}}
			{...props}
		/>
	);
};

const styles = {
	// Adds some spacing from the popover content
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
