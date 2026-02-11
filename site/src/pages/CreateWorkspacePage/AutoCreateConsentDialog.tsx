import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type { AutofillBuildParameter } from "utils/richParameters";

interface AutoCreateConsentDialogProps {
	open: boolean;
	autofillParameters: AutofillBuildParameter[];
	onConfirm: () => void;
	onDeny: () => void;
}

export const AutoCreateConsentDialog: FC<AutoCreateConsentDialogProps> = ({
	open,
	autofillParameters,
	onConfirm,
	onDeny,
}) => {
	return (
		<Dialog open={open}>
			<DialogContent
				onPointerDownOutside={(e) => e.preventDefault()}
				onEscapeKeyDown={(e) => e.preventDefault()}
				className="max-w-2xl overflow-hidden min-w-0"
			>
				<DialogHeader>
					<DialogTitle>
						<TriangleAlertIcon className="size-icon-lg text-content-warning inline-block align-text-bottom mr-2" />
						Warning: Automatic Workspace Creation
					</DialogTitle>
					<DialogDescription>
						A link is attempting to automatically create a workspace using the
						following external configurations. Running scripts from untrusted
						sources can be dangerous.
					</DialogDescription>
				</DialogHeader>

				{autofillParameters.length > 0 && (
					<div className="flex min-w-0 flex-col gap-2">
						<span className="text-sm font-semibold text-content-primary">
							Parameters:
						</span>
						<code className="block whitespace-pre overflow-x-auto">
							{autofillParameters
								.map((p) => `${p.name}: ${p.value}`)
								.join("\n")}
						</code>
					</div>
				)}

				<DialogFooter>
					<Button variant="outline" onClick={onDeny}>
						Cancel
					</Button>
					<Button variant="destructive" onClick={onConfirm}>
						Confirm and Create
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
