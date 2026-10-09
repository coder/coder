import {
	ArrowUpIcon,
	CheckIcon,
	MicIcon,
	PencilIcon,
	SquareIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import { use, useRef } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { chatAttachmentAcceptAttribute } from "../utils/chatAttachments";
import type { ChatSlashCommand } from "../utils/slashCommands";
import {
	type AgentComposerBindings,
	ComposerContext,
	ComposerRefsContext,
	useAgentComposer,
} from "./AgentComposer/context";
import { Frame } from "./AgentComposer/Frame";
import { ComposerFilePreviews } from "./AgentComposer/useComposerFiles";
import { useComposerRuntime } from "./AgentComposer/useComposerRuntime";
import { AttachmentPreview } from "./AttachmentPreview";
import {
	ChatMessageInput,
	type ChatMessageInputRef,
} from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

export {
	type AgentComposerBindings,
	useAgentComposer,
} from "./AgentComposer/context";

function Provider({
	bindings,
	children,
}: {
	bindings: AgentComposerBindings;
	children: React.ReactNode;
}) {
	const editorRef = useRef<ChatMessageInputRef>(null);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const { context, previews } = useComposerRuntime(
		bindings,
		editorRef,
		fileInputRef,
	);
	return (
		<ComposerContext value={context}>
			<ComposerRefsContext value={{ editorRef, fileInputRef }}>
				{children}
			</ComposerRefsContext>
			<ComposerFilePreviews {...previews} />
		</ComposerContext>
	);
}

function Editor({
	placeholder = "Type a message...",
	workspaceSkills,
	slashCommands,
	hasWorkspace,
}: {
	placeholder?: string;
	workspaceSkills?: readonly SkillMetadata[];
	slashCommands?: readonly ChatSlashCommand[];
	hasWorkspace: boolean;
}) {
	const { state, actions, meta } = useAgentComposer();
	const refs = use(ComposerRefsContext);
	if (!refs)
		throw new Error("Editor must be used inside AgentComposer.Provider");
	const { editorRef } = refs;
	return (
		<ChatMessageInput
			ref={editorRef}
			onFilePaste={state.canAttachFiles ? actions.filePaste : undefined}
			acceptFilePasteWhileDisabled={state.isLoading && !state.isReadOnly}
			onPaste={actions.resetPromptCycle}
			aria-label="Chat message"
			aria-describedby={state.warning ? meta.warningId : undefined}
			className="min-h-[60px] sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
			placeholder={placeholder}
			initialValue={meta.initialValue}
			initialEditorState={meta.initialEditorState}
			remountKey={meta.remountKey}
			onChange={actions.contentChange}
			onKeyDown={actions.editorKeyDown}
			onEnter={actions.submit}
			sendShortcut={meta.sendShortcut}
			disabled={state.isReadOnly || state.isLoading}
			hasWorkspace={hasWorkspace}
			workspaceSkills={workspaceSkills}
			slashCommands={slashCommands}
			skillsMenuAnchor={meta.composerElement}
		/>
	);
}

function Attachments() {
	const { state, actions, meta } = useAgentComposer();
	const refs = use(ComposerRefsContext);
	if (!refs)
		throw new Error("Attachments must be used inside AgentComposer.Provider");
	const { fileInputRef } = refs;
	return (
		<>
			{meta.onRemoveAttachment && (
				<AttachmentPreview
					attachments={meta.attachments}
					onRemove={meta.onRemoveAttachment}
					uploadStates={meta.uploadStates}
					previewUrls={meta.previewUrls}
					onPreview={actions.imagePreview}
					textContents={meta.textContents}
					onTextPreview={actions.textPreview}
					onInlineText={actions.inlineText}
				/>
			)}
			{meta.workspaceUploads && (
				<WorkspaceUploadPreview
					uploads={meta.workspaceUploads.uploads}
					onRemove={meta.workspaceUploads.onRemove}
				/>
			)}
			{/* Allow all workspace upload types so routeFiles can explain refusals on iOS. */}
			{state.canAttachFiles && (
				<input
					ref={fileInputRef}
					type="file"
					data-testid="chat-attachment-file-input"
					multiple
					accept={
						meta.workspaceUploads ? undefined : chatAttachmentAcceptAttribute
					}
					onChange={actions.fileSelect}
					className="hidden"
				/>
			)}
		</>
	);
}

function Toolbar({ children }: { children: React.ReactNode }) {
	return (
		<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
			{children}
		</div>
	);
}

function VoiceInput() {
	const { state, actions } = useAgentComposer();
	if (!state.speechSupported) return null;
	return (
		<>
			<Button
				type="button"
				variant="subtle"
				size="icon"
				className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
				onClick={
					state.speechRecording
						? actions.cancelRecording
						: actions.startRecording
				}
				disabled={state.isDisabled}
				aria-label={
					state.speechRecording ? "Cancel voice input" : "Voice input"
				}
			>
				{state.speechRecording ? <XIcon /> : <MicIcon strokeWidth={1.5} />}
			</Button>
			{state.speechError && !state.speechRecording && (
				<span className="text-2xs text-content-destructive" role="alert">
					{state.speechError === "not-allowed"
						? "Mic access denied"
						: "Voice input failed"}
				</span>
			)}
		</>
	);
}

function PrimaryAction() {
	const { state, actions, meta } = useAgentComposer();
	return (
		<>
			{state.showSendButton && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							size="icon"
							variant="default"
							className="size-7 rounded-full transition-colors [&>svg]:size-5! [&>svg]:p-0"
							onClick={
								state.speechRecording ? actions.acceptRecording : actions.submit
							}
							disabled={state.speechRecording ? false : !state.canSend}
							aria-keyshortcuts={meta.sendButtonKeyShortcuts}
						>
							{state.isLoading && !state.isInterruptPending ? (
								<Spinner size="sm" loading aria-hidden="true" />
							) : state.speechRecording ? (
								<CheckIcon />
							) : (
								<ArrowUpIcon />
							)}
							<span className="sr-only">
								{state.speechRecording
									? "Accept voice input"
									: meta.sendButtonLabel}
							</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">
						{state.speechRecording
							? "Accept voice input"
							: meta.sendButtonTooltip}
					</TooltipContent>
				</Tooltip>
			)}
			{state.showStopButton && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							size="icon"
							variant="default"
							className="size-7 rounded-full transition-colors [&>svg]:size-3! [&>svg]:p-0"
							onClick={actions.interrupt}
							disabled={state.isInterruptPending}
						>
							<SquareIcon className="fill-current" />
							<span className="sr-only">Stop</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">
						{state.isInterruptPending ? "Interrupting…" : "Stop"}
					</TooltipContent>
				</Tooltip>
			)}
			{state.isInterruptPending && state.isStreaming && (
				<span role="status" className="sr-only">
					Interrupting. Waiting for the agent to stop.
				</span>
			)}
		</>
	);
}

function InvisibleCharacterWarning() {
	const { state } = useAgentComposer();
	const { invisibleCharCount } = state;
	if (invisibleCharCount === 0) return null;
	// Unlike admin/user prompt textareas (which strip invisible chars
	// server-side on save), chat messages are free-form user input. Warn
	// without silently mutating them so users can review hidden content.
	// This guards against social engineering that tricks users into pasting
	// prompts with hidden LLM instructions encoded as zero-width characters.
	return (
		<div className="px-3 pb-1">
			<Alert severity="warning">
				<AlertDescription>
					This message contains {invisibleCharCount} invisible Unicode character
					{invisibleCharCount !== 1 ? "s" : ""} that could hide content. Review
					carefully before sending.
				</AlertDescription>
			</Alert>
		</div>
	);
}

function Warning() {
	const { state, meta } = useAgentComposer();
	if (!state.warning) return null;
	return (
		<div
			id={meta.warningId}
			className="flex items-start gap-1.5 border-b border-border/70 px-3 py-1.5 text-xs font-medium text-content-warning"
		>
			<TriangleAlertIcon className="mt-px size-3.5 shrink-0" />
			{state.warning}
		</div>
	);
}

function EditBanner() {
	const { state, actions } = useAgentComposer();
	if (!state.isEditingHistoryMessage) return null;
	return (
		<div className="flex items-center justify-between border-b border-border/70 px-3 py-1.5">
			<span className="flex items-center gap-1.5 text-xs font-medium text-content-warning">
				<PencilIcon className="size-3.5" />
				Editing will delete all subsequent messages and restart the conversation
				here.
			</span>
			<Button
				type="button"
				variant="subtle"
				size="icon"
				aria-label="Cancel editing"
				onClick={actions.cancelHistoryEdit}
				disabled={state.isLoading}
				className="size-6 rounded text-content-warning hover:text-content-primary"
			>
				<XIcon className="size-3.5" />
			</Button>
		</div>
	);
}

/** Composable UI for the agent chat draft runtime. */
export const AgentComposer = {
	Provider,
	Frame,
	Editor,
	Attachments,
	Toolbar,
	VoiceInput,
	PrimaryAction,
	InvisibleCharacterWarning,
	Warning,
	EditBanner,
};
