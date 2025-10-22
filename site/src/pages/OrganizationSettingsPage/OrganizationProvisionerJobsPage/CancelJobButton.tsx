import type { ProvisionerJob } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { BanIcon } from "lucide-react";
import { type FC, useState } from "react";
import { CancelJobConfirmationDialog } from "./CancelJobConfirmationDialog";

const CANCELLABLE = ["pending", "running"];

type CancelJobButtonProps = {
	job: ProvisionerJob;
};

export const CancelJobButton: FC<CancelJobButtonProps> = ({ job }) => {
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const isCancellable = CANCELLABLE.includes(job.status);

	return (
		<>
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							disabled={!isCancellable}
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

			<CancelJobConfirmationDialog
				open={isDialogOpen}
				job={job}
				onClose={() => {
					setIsDialogOpen(false);
				}}
			/>
		</>
	);
};
