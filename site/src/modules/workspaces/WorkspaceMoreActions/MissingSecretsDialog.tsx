import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type MissingSecretsDialogProps = {
	open: boolean;
	onClose: () => void;
	count: number;
};

export const MissingSecretsDialog: FC<MissingSecretsDialogProps> = ({
	open,
	onClose,
	count,
}) => {
	// TODO: wire this up once the user secrets page exists. PLAT-102
	// builds the destination page; once that lands we can replace this
	// stub with the actual navigation and drop the Coming soon tooltip.
	// The button is rendered now for layout parity with the parameter
	// dialog so the dialog ships with the rest of the secrets work.
	const handleGoToSecrets = () => {
		onClose();
	};

	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Missing required secrets</DialogTitle>
					<DialogDescription>
						This template requires{" "}
						<strong className="text-content-primary">
							{count} secret{count === 1 ? "" : "s"}
						</strong>{" "}
						that you haven't created yet.
					</DialogDescription>
					<DialogDescription>
						Would you like to go to the user secrets page to review and update
						secrets before continuing?
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button onClick={onClose} variant="outline">
						Cancel
					</Button>
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild>
								<span>
									<Button
										onClick={handleGoToSecrets}
										disabled
										aria-disabled={true}
									>
										Go to user secrets
									</Button>
								</span>
							</TooltipTrigger>
							<TooltipContent>Coming soon</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
