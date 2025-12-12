import { useTheme } from "@emotion/react";
import { API } from "api/api";
import { Button } from "components/Button/Button";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import type { FC } from "react";
import { useMutation } from "react-query";

export const Troubleshooting: FC = () => {
	const { mutate: sendTestNotificationApi, isPending } = useMutation({
		mutationFn: API.postTestNotification,
		onSuccess: () => displaySuccess("Test notification sent"),
		onError: () => displayError("Failed to send test notification"),
	});

	const theme = useTheme();
	return (
		<>
			<div
				css={{
					color: theme.palette.text.secondary,
				}}
				className="text-sm leading-relaxed mb-4"
			>
				Send a test notification to troubleshoot your notification settings.
			</div>
			<div>
				<span>
					<Button
						variant="outline"
						size="sm"
						disabled={isPending}
						onClick={() => {
							sendTestNotificationApi();
						}}
					>
						<Spinner loading={isPending} />
						Send notification
					</Button>
				</span>
			</div>
		</>
	);
};
