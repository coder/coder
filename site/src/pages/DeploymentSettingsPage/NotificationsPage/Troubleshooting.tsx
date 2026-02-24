import { useTheme } from "@emotion/react";
import { API } from "api/api";
import { getErrorDetail } from "api/errors";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import type { FC } from "react";
import { useMutation } from "react-query";
import { toast } from "sonner";

export const Troubleshooting: FC = () => {
	const { mutate: sendTestNotificationApi, isPending } = useMutation({
		mutationFn: API.postTestNotification,
		onSuccess: () => toast.success("Test notification sent."),
		onError: (error) =>
			toast.error("Failed to send test notification.", {
				description: getErrorDetail(error),
			}),
	});

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
