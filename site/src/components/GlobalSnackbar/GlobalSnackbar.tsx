import { type FC, useCallback, useState } from "react";
import { useCustomEvent } from "hooks/events";
import type { CustomEventListener } from "utils/events";
import { EnterpriseSnackbar } from "./EnterpriseSnackbar";
import { ErrorIcon } from "../Icons/ErrorIcon";
import { Typography } from "../Typography/Typography";
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

  const handleNotification = useCallback<CustomEventListener<NotificationMsg>>(
    (event) => {
      setNotification(event.detail);
      setOpen(true);
    },
    [],
  );

  useCustomEvent(SnackbarEventType, handleNotification);

  const renderAdditionalMessage = (msg: AdditionalMessage, idx: number) => {
    if (isNotificationText(msg)) {
      return (
        <Typography
          key={idx}
          gutterBottom
          variant="body2"
          css={styles.messageSubtitle}
        >
          {msg}
        </Typography>
      );
    } else if (isNotificationTextPrefixed(msg)) {
      return (
        <Typography
          key={idx}
          gutterBottom
          variant="body2"
          css={styles.messageSubtitle}
        >
          <strong>{msg.prefix}:</strong> {msg.text}
        </Typography>
      );
    } else if (isNotificationList(msg)) {
      return (
        <ul css={styles.list} key={idx}>
          {msg.map((item, idx) => (
            <li key={idx}>
              <Typography variant="body2" css={styles.messageSubtitle}>
                {item}
              </Typography>
            </li>
          ))}
        </ul>
      );
    }
    return null;
  };

  if (!notification) {
    return null;
  }

  return (
    <EnterpriseSnackbar
      key={notification.msg}
      open={open}
      variant={variantFromMsgType(notification.msgType)}
      message={
        <div css={styles.messageWrapper}>
          {notification.msgType === MsgType.Error && (
            <ErrorIcon css={styles.errorIcon} />
          )}
          <div css={styles.message}>
            <Typography variant="body1" css={styles.messageTitle}>
              {notification.msg}
            </Typography>
            {notification.additionalMsgs &&
              notification.additionalMsgs.map(renderAdditionalMessage)}
          </div>
        </div>
      }
      onClose={() => setOpen(false)}
      autoHideDuration={notification.msgType === MsgType.Error ? 22000 : 6000}
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "right",
      }}
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
