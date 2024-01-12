import { FC, ReactNode } from "react";
import { Pill } from "components/Pill/Pill";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";
import { Interpolation, Theme, useTheme } from "@emotion/react";
import Button, { ButtonProps } from "@mui/material/Button";
import { ThemeRole } from "theme/experimental";
import { AlertProps } from "components/Alert/Alert";

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
  isDefaultOpen?: boolean;
};

export const Notifications: FC<NotificationsProps> = ({
  items,
  severity,
  icon,
  isDefaultOpen,
}) => {
  const theme = useTheme();

  return (
    <Popover mode="hover" isDefaultOpen={isDefaultOpen}>
      <PopoverTrigger>
        <div css={styles.pillContainer}>
          <NotificationPill items={items} severity={severity} icon={icon} />
        </div>
      </PopoverTrigger>
      <PopoverContent
        horizontal="right"
        css={{
          "& .MuiPaper-root": {
            borderColor: theme.experimental.roles[severity].outline,
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

const NotificationPill = (props: NotificationsProps) => {
  const { items, severity, icon } = props;
  const popover = usePopover();

  return (
    <Pill
      icon={icon}
      css={(theme) => ({
        "& svg": { color: theme.experimental.roles[severity].outline },
        borderColor: popover.isOpen
          ? theme.experimental.roles[severity].outline
          : undefined,
      })}
    >
      {items.length}
    </Pill>
  );
};

const NotificationItem: FC<{ notification: NotificationItem }> = (props) => {
  const { notification } = props;

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
        padding: 0,
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

    "&:first-child": {
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
