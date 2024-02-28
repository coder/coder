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
  const [notificationMsg, setNotificationMsg] = useState<NotificationMsg>();
  useCustomEvent<NotificationMsg>(SnackbarEventType, (event) => {
    setNotificationMsg(event.detail);
  });

  const hasNotification = notificationMsg !== undefined;
  if (!hasNotification) {
    return null;
  }

  return (
    <EnterpriseSnackbar
      key={notificationMsg.msg}
      open={hasNotification}
      variant={variantFromMsgType(notificationMsg.msgType)}
      onClose={() => setNotificationMsg(undefined)}
      autoHideDuration={
        notificationMsg.msgType === MsgType.Error ? 22000 : 6000
      }
      anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
      message={
        <div css={{ display: "flex" }}>
          {notificationMsg.msgType === MsgType.Error && (
            <ErrorIcon css={styles.errorIcon} />
          )}

          <div css={{ maxWidth: 670 }}>
            <span css={styles.messageTitle}>{notificationMsg.msg}</span>

            {notificationMsg.additionalMsgs &&
              notificationMsg.additionalMsgs.map((msg, index) => (
                <AdditionalMessageDisplay key={index} message={msg} />
              ))}
          </div>
        </div>
      }
    />
  );
};

interface AdditionalMessageDisplayProps {
  message: AdditionalMessage;
}

const AdditionalMessageDisplay: FC<AdditionalMessageDisplayProps> = ({
  message,
}) => {
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
      <ul css={{ paddingLeft: 0 }}>
        {message.map((item, idx) => (
          <li key={idx}>
            <span css={styles.messageSubtitle}>{item}</span>
          </li>
        ))}
      </ul>
    );
  }

  return null;
};

const styles = {
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
