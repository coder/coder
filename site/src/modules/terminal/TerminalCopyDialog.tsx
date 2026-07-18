import type { Terminal } from "@xterm/xterm";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

export const getVisibleTerminalText = (terminal: Terminal): string => {
	const lines: string[] = [];
	const firstLine = terminal.buffer.active.viewportY;
	const lastLine = firstLine + terminal.rows;

	for (let index = firstLine; index < lastLine; index++) {
		lines.push(
			terminal.buffer.active.getLine(index)?.translateToString(true) ?? "",
		);
	}

	while (lines.at(-1) === "") {
		lines.pop();
	}

	return lines.join("\n");
};

type TerminalCopyDialogProps = {
	open: boolean;
	text: string;
	onOpenChange: (open: boolean) => void;
	onCopy: () => void;
	onClose: () => void;
};

export const TerminalCopyDialog = ({
	open,
	text,
	onOpenChange,
	onCopy,
	onClose,
}: TerminalCopyDialogProps) => {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					onClose();
				}}
			>
				<DialogHeader>
					<DialogTitle>Copy terminal output</DialogTitle>
					<DialogDescription>
						Select text below or copy all visible terminal output.
					</DialogDescription>
				</DialogHeader>
				<textarea
					aria-label="Visible terminal output"
					className="min-h-64 w-full resize-y rounded-md border border-solid border-border-default bg-surface-secondary p-3 font-mono text-sm text-content-primary"
					readOnly
					spellCheck={false}
					value={text}
				/>
				<DialogFooter>
					<Button onClick={onCopy}>Copy</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
