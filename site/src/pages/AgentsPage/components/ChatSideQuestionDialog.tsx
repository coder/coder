import type { FC } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { Response } from "./ChatElements/Response";

export type ChatSideQuestionDialogState =
	| { status: "closed" }
	| { status: "streaming"; question: string; answer: string }
	| { status: "success"; question: string; answer: string }
	| { status: "error"; question: string; message: string; answer?: string };

interface ChatSideQuestionDialogProps {
	state: ChatSideQuestionDialogState;
	onClose: () => void;
}

export const ChatSideQuestionDialog: FC<ChatSideQuestionDialogProps> = ({
	state,
	onClose,
}) => {
	if (state.status === "closed") {
		return null;
	}

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[85vh] max-w-[90vw] w-full sm:w-fit sm:min-w-[420px] flex flex-col gap-0 p-0"
				aria-describedby={undefined}
			>
				<DialogTitle className="px-4 py-3 border-b border-border-default text-sm font-medium">
					Side question
				</DialogTitle>
				<div className="flex max-h-[calc(85vh-3rem)] flex-col gap-4 overflow-auto p-4">
					<div>
						<div className="mb-1 text-xs font-medium text-content-secondary">
							Question
						</div>
						<p className="m-0 whitespace-pre-wrap text-sm text-content-primary">
							{state.question}
						</p>
					</div>
					{state.status === "streaming" && state.answer === "" && (
						<div className="flex items-center gap-2 text-sm text-content-secondary">
							<Spinner size="sm" loading aria-hidden="true" />
							Answering side question...
						</div>
					)}
					{(state.status === "streaming" || state.status === "success") &&
						state.answer !== "" && (
							<div>
								<div className="mb-1 text-xs font-medium text-content-secondary">
									Answer
								</div>
								<Response
									className="max-w-3xl"
									streaming={state.status === "streaming"}
								>
									{state.answer}
								</Response>
							</div>
						)}
					{state.status === "error" && (
						<Alert severity="error">
							<AlertDescription>{state.message}</AlertDescription>
						</Alert>
					)}
					<div className="flex justify-end">
						<Button type="button" variant="outline" size="sm" onClick={onClose}>
							{state.status === "streaming" ? "Cancel" : "Dismiss"}
						</Button>
					</div>
				</div>
			</DialogContent>
		</Dialog>
	);
};
