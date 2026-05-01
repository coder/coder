import type { FC } from "react";
import { useMutation } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorDetail } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";

type TroubleshootingProps = {
	canEdit?: boolean;
};

export const Troubleshooting: FC<TroubleshootingProps> = ({
	canEdit = true,
}) => {
	const { mutate: sendTestNotificationApi, isPending } = useMutation({
		mutationFn: API.postTestNotification,
		onSuccess: () => toast.success("Test notification sent."),
		onError: (error) =>
			toast.error("Failed to send test notification.", {
				description: getErrorDetail(error),
			}),
	});

	return (
		<>
			<div className="text-sm text-content-secondary leading-[160%] mb-4">
				Send a test notification to troubleshoot your notification settings.{" "}
			</div>
			<div>
				<span>
					<Button
						variant="outline"
						size="sm"
						disabled={isPending || !canEdit}
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
