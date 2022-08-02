import { makeStyles } from "@material-ui/core/styles"
import React, { useCallback, useState } from "react"
import { useCustomEvent } from "../../hooks/events"
import { CustomEventListener } from "../../util/events"
import { EnterpriseSnackbar } from "../EnterpriseSnackbar/EnterpriseSnackbar"
import { ErrorIcon } from "../Icons/ErrorIcon"
import { Typography } from "../Typography/Typography"
import {
  AdditionalMessage,
  isNotificationList,
  isNotificationText,
  isNotificationTextPrefixed,
  MsgType,
  NotificationMsg,
  SnackbarEventType,
} from "./utils"

const variantFromMsgType = (type: MsgType) => {
  if (type === MsgType.Error) {
    return "error"
  } else if (type === MsgType.Success) {
    return "success"
  } else {
    return "info"
  }
}

export const GlobalSnackbar: React.FC<React.PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  const [open, setOpen] = useState<boolean>(false)
  const [notification, setNotification] = useState<NotificationMsg>()

  const handleNotification = useCallback<CustomEventListener<NotificationMsg>>((event) => {
    setNotification(event.detail)
    setOpen(true)
  }, [])

  useCustomEvent(SnackbarEventType, handleNotification)

  const renderAdditionalMessage = (msg: AdditionalMessage, idx: number) => {
    if (isNotificationText(msg)) {
      return (
        <Typography key={idx} gutterBottom variant="body2" className={styles.messageSubtitle}>
          {msg}
        </Typography>
      )
    } else if (isNotificationTextPrefixed(msg)) {
      return (
        <Typography key={idx} gutterBottom variant="body2" className={styles.messageSubtitle}>
          <strong>{msg.prefix}:</strong> {msg.text}
        </Typography>
      )
    } else if (isNotificationList(msg)) {
      return (
        <ul className={styles.list} key={idx}>
          {msg.map((item, idx) => (
            <li key={idx}>
              <Typography variant="body2" className={styles.messageSubtitle}>
                {item}
              </Typography>
            </li>
          ))}
        </ul>
      )
    }
    return null
  }

  if (!notification) {
    return null
  }

  return (
    <EnterpriseSnackbar
      key={notification.msg}
      open={open}
      variant={variantFromMsgType(notification.msgType)}
      message={
        <div className={styles.messageWrapper}>
          {notification.msgType === MsgType.Error && <ErrorIcon className={styles.errorIcon} />}
          <div className={styles.message}>
            <Typography variant="body1" className={styles.messageTitle}>
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
  )
}

const useStyles = makeStyles((theme) => ({
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
    marginTop: theme.spacing(1.5),
  },
  errorIcon: {
    color: theme.palette.error.contrastText,
    marginRight: theme.spacing(2),
  },
}))
