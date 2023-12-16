import { dispatchCustomEvent } from "utils/events";

///////////////////////////////////////////////////////////////////////////////
// Notification Types
///////////////////////////////////////////////////////////////////////////////

export enum MsgType {
  Info,
  Success,
  Error,
}

/**
 * Display a prefixed paragraph inside a notification.
 */
export type NotificationTextPrefixed = {
  prefix: string;
  text: string;
};

export type AdditionalMessage = NotificationTextPrefixed | string[] | string;

export const isNotificationText = (msg: AdditionalMessage): msg is string => {
  return !Array.isArray(msg) && typeof msg === "string";
};

export const isNotificationTextPrefixed = (
  msg: AdditionalMessage | null,
): msg is NotificationTextPrefixed => {
  if (msg) {
    return (
      typeof msg !== "string" &&
      Object.prototype.hasOwnProperty.call(msg, "prefix")
    );
  }
  return false;
};

export const isNotificationList = (msg: AdditionalMessage): msg is string[] => {
  return Array.isArray(msg);
};

export interface NotificationMsg {
  msgType: MsgType;
  msg: string;
  additionalMsgs?: AdditionalMessage[];
}

export const SnackbarEventType = "coder:notification";

///////////////////////////////////////////////////////////////////////////////
// Notification Functions
///////////////////////////////////////////////////////////////////////////////

function dispatchNotificationEvent(
  msgType: MsgType,
  msg: string,
  additionalMsgs?: AdditionalMessage[],
) {
  dispatchCustomEvent<NotificationMsg>(SnackbarEventType, {
    msgType,
    msg,
    additionalMsgs,
  });
}

export const displaySuccess = (msg: string, additionalMsg?: string): void => {
  dispatchNotificationEvent(
    MsgType.Success,
    msg,
    additionalMsg ? [additionalMsg] : undefined,
  );
};

export const displayError = (msg: string, additionalMsg?: string): void => {
  dispatchNotificationEvent(
    MsgType.Error,
    msg,
    additionalMsg ? [additionalMsg] : undefined,
  );
};
