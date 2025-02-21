import { useTheme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import { API } from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { useMutation } from "react-query";

export const Troubleshooting: FC = () => {
	const { mutate: sendTestNotificationApi, isLoading } = useMutation(
		API.postTestNotification,
		{
			onSuccess: () => displaySuccess("Test notification sent"),
			onError: () => displayError("Failed to send test notification"),
		},
	);

	const theme = useTheme();
	return (
		<>
			<div
				css={{
					fontSize: 14,
					color: theme.palette.text.secondary,
					lineHeight: "160%",
					marginBottom: 16,
				}}
			>
				Send a test notification to troubleshoot your notification settings.
			</div>
			<div>
				<span>
					<LoadingButton
						variant="outlined"
						loading={isLoading}
						size="small"
						css={{ minWidth: "auto", aspectRatio: "1" }}
						onClick={() => {
							sendTestNotificationApi();
						}}
					>
						Send notification
					</LoadingButton>
				</span>
			</div>
		</>
	);
};
