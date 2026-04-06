import type { FC } from "react";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";

interface TextPreviewDialogProps {
	content: string;
	fileName?: string;
	onClose: () => void;
}

export const TextPreviewDialog: FC<TextPreviewDialogProps> = ({
	content,
	fileName,
	onClose,
}) => {
	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[85vh] max-w-[90vw] w-full sm:w-fit sm:min-w-[400px] flex flex-col gap-0 p-0"
				aria-describedby={undefined}
			>
				<DialogTitle className="px-4 py-3 border-b border-border-default text-sm font-medium">
					{fileName ?? "Pasted text"}
				</DialogTitle>
				<div className="overflow-auto p-4 max-h-[calc(85vh-3rem)]">
					<pre className="whitespace-pre-wrap break-words text-sm text-content-primary font-mono m-0">
						{content}
					</pre>
				</div>
			</DialogContent>
		</Dialog>
	);
};
