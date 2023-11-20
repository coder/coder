import { type FC, useState } from "react";
import { useCustomEvent } from "hooks/events";
import { EnterpriseSnackbar } from "./EnterpriseSnackbar";
import { ErrorIcon } from "../Icons/ErrorIcon";
import {
  type AdditionalMessage,
  isNotificationList,
  isNotificationText,
  isNotificationTextPrefixed,
  MsgType,
  type NotificationMsg,
  SnackbarEventType,
} from "./utils";
import { type Interpolation, type Theme } from "@emotion/react";

const variantFromMsgType = (type: MsgType) => {
  if (type === MsgType.Error) {
    return "error";
  } else if (type === MsgType.Success) {
    return "success";
  } else {
    return "info";
  }
};

export const GlobalSnackbar: FC = () => {
  const [open, setOpen] = useState<boolean>(false);
  const [notification, setNotification] = useState<NotificationMsg>();

  useCustomEvent<NotificationMsg>(SnackbarEventType, (event) => {
    setNotification(event.detail);
    setOpen(true);
  });

  if (!notification) {
    return null;
  }

  return (
    <EnterpriseSnackbar
      key={notification.msg}
      open={open}
      variant={variantFromMsgType(notification.msgType)}
      onClose={() => setOpen(false)}
      autoHideDuration={notification.msgType === MsgType.Error ? 22000 : 6000}
      anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
      message={
        <div css={styles.messageWrapper}>
          {notification.msgType === MsgType.Error && (
            <ErrorIcon css={styles.errorIcon} />
          )}

          <div css={styles.message}>
            <span css={styles.messageTitle}>{notification.msg}</span>

            {notification.additionalMsgs &&
              notification.additionalMsgs.map((msg, index) => (
                <AdditionalMessageDisplay key={index} message={msg} />
              ))}
          </div>
        </div>
      }
    />
  );
};

const styles = {
  list: {
    paddingLeft: 0,
  },
  messageWrapper: {
    display: "flex",
  },
  message: {
    maxWidth: 670,
  },
  messageTitle: {
    fontSize: 14,
    fontWeight: 600,
  },
  messageSubtitle: {
    marginTop: 12,
  },
  errorIcon: (theme) => ({
    color: theme.palette.error.contrastText,
    marginRight: 16,
  }),
} satisfies Record<string, Interpolation<Theme>>;

function AdditionalMessageDisplay({ message }: { message: AdditionalMessage }) {
  if (isNotificationText(message)) {
    return <span css={styles.messageSubtitle}>{message}</span>;
  }

  if (isNotificationTextPrefixed(message)) {
    return (
      <span css={styles.messageSubtitle}>
        <strong>{message.prefix}:</strong> {message.text}
      </span>
    );
  }

  if (isNotificationList(message)) {
    return (
      <ul css={styles.list}>
        {message.map((item, idx) => (
          <li key={idx}>
            <span css={styles.messageSubtitle}>{item}</span>
          </li>
        ))}
      </ul>
    );
  }

  return null;
}
