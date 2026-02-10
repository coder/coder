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
				className="flex max-w-2xl flex-col gap-4"
			>
				<DialogHeader className="flex-row items-center gap-2 space-y-0">
					<TriangleAlertIcon className="size-icon-lg text-content-warning" />
					<DialogTitle className="m-0">
						Warning: Automatic Workspace Creation
					</DialogTitle>
				</DialogHeader>

				<DialogDescription>
					A link is attempting to automatically create a workspace using the
					following external configurations. Running scripts from untrusted
					sources can be dangerous.
				</DialogDescription>

				{autofillParameters.length > 0 && (
					<div className="flex flex-col gap-2">
						<span className="text-sm font-semibold text-content-primary">
							Parameters:
						</span>
						<code className="whitespace-pre overflow-x-auto">
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
