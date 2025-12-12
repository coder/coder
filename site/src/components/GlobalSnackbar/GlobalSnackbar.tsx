import { useCustomEvent } from "hooks/events";
import { type FC, useState } from "react";
import { ErrorIcon } from "../Icons/ErrorIcon";
import { EnterpriseSnackbar } from "./EnterpriseSnackbar";
import {
	type AdditionalMessage,
	isNotificationList,
	isNotificationText,
	isNotificationTextPrefixed,
	MsgType,
	type NotificationMsg,
	SnackbarEventType,
} from "./utils";

const variantFromMsgType = (type: MsgType) => {
	if (type === MsgType.Error) {
		return "error";
	}

	if (type === MsgType.Success) {
		return "success";
	}
	return "info";
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
				<div className="flex">
					{notificationMsg.msgType === MsgType.Error && (
						<ErrorIcon className={classNames.errorIcon} />
					)}

					<div className="max-w-[670px] flex flex-col">
						<span className={classNames.messageTitle}>
							{notificationMsg.msg}
						</span>

						{notificationMsg.additionalMsgs?.map((msg, index) => (
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
		return <span className={classNames.messageSubtitle}>{message}</span>;
	}

	if (isNotificationTextPrefixed(message)) {
		return (
			<span className={classNames.messageSubtitle}>
				<strong>{message.prefix}:</strong> {message.text}
			</span>
		);
	}

	if (isNotificationList(message)) {
		return (
			<ul className="pl-0">
				{message.map((item, idx) => (
					<li key={idx}>
						<span className={classNames.messageSubtitle}>{item}</span>
					</li>
				))}
			</ul>
		);
	}

	return null;
};

const classNames = {
	messageTitle: "text-sm font-semibold",
	messageSubtitle: "mt-1 first-letter:uppercase",
	errorIcon: "text-content-destructive mr-4",
};
