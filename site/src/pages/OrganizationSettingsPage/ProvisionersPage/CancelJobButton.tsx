import { useState, type FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Button } from "components/Button/Button";
import { BanIcon } from "lucide-react";
import type { ProvisionerJob } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";

type CancelJobButtonProps = {
	job: ProvisionerJob;
};

export const CancelJobButton: FC<CancelJobButtonProps> = ({ job }) => {
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const cancellable = ["pending", "running"].includes(job.status);

	return (
		<>
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							disabled={!cancellable}
							aria-label="Cancel job"
							size="icon"
							variant="outline"
							onClick={() => {
								setIsDialogOpen(true);
							}}
						>
							<BanIcon />
						</Button>
					</TooltipTrigger>
					<TooltipContent>Cancel job</TooltipContent>
				</Tooltip>
			</TooltipProvider>

			<ConfirmDialog
				type="delete"
				onClose={(): void => {
					setIsDialogOpen(false);
				}}
				open={isDialogOpen}
				title="Cancel provisioner job"
				description={`Are you sure you want to cancel the provisioner job "${job.id}"? This operation will result in the associated workspaces not getting created.`}
				confirmText="Confirm"
				cancelText="Discard"
			/>
		</>
	);
};
