import {
	ArrowUpIcon,
	CheckIcon,
	MicIcon,
	PencilIcon,
	SquareIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { useState } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ChatSlashCommand } from "../utils/slashCommands";
import type { AgentComposerBindings } from "./AgentComposer/context";
import {
	ComposerContext,
	type ComposerContextValue,
	useAgentComposer,
} from "./AgentComposer/context";
import { Frame } from "./AgentComposer/Frame";
import { useComposerRuntime } from "./AgentComposer/useComposerRuntime";
import { AttachmentPreview } from "./AttachmentPreview";
import { ChatMessageInput } from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import { ImageLightbox } from "./ImageLightbox";
import { TextPreviewDialog } from "./TextPreviewDialog";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

export {
	type ComposerChatBindings,
	type ComposerContextValue,
	type ComposerDraftBindings,
	type ComposerEditorBindings,
	type ComposerFileBindings,
	useAgentComposer,
} from "./AgentComposer/context";

function Provider({
	children,
	...value
}: ComposerContextValue & { children: React.ReactNode }) {
	return <ComposerContext value={value}>{children}</ComposerContext>;
}

/** Supplies the default draft runtime through the injectable composer contract. */
export function AgentComposerRuntimeProvider({
	bindings,
	children,
	needsSetup = false,
}: {
	bindings: AgentComposerBindings;
	children: React.ReactNode;
	needsSetup?: boolean;
}) {
	const value = useComposerRuntime(bindings, needsSetup);

	return <Provider {...value}>{children}</Provider>;
}

/** Editor affordances selected by an assembly, separate from document state. */
export type ComposerEditorProps = {
	placeholder?: string;
	workspaceSkills?: readonly SkillMetadata[];
	slashCommands?: readonly ChatSlashCommand[];
	hasWorkspace?: boolean;
};

function Editor({
	placeholder = "Type a message...",
	workspaceSkills,
	slashCommands,
	hasWorkspace = false,
}: ComposerEditorProps) {
	const { state, actions, meta } = useAgentComposer();
	const { editorRef } = meta;

	return (
		<ChatMessageInput
			ref={editorRef}
			onFilePaste={
				state.canAttachFiles ? (file) => actions.attachFiles([file]) : undefined
			}
			acceptFilePasteWhileDisabled={state.isLoading}
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
	const { state, actions } = useAgentComposer();
	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<{
		content: string;
		fileName: string;
		mediaType: string;
	} | null>(null);
	const files = state.files;

	if (!files) {
		return null;
	}

	return (
		<>
			<AttachmentPreview
				attachments={files.attachments}
				onRemove={actions.removeAttachment}
				uploadStates={files.uploadStates}
				previewUrls={files.previewUrls}
				onPreview={setPreviewImage}
				textContents={files.textContents}
				onTextPreview={(content, fileName, mediaType) => {
					setPreviewText({ content, fileName, mediaType });
				}}
				onInlineText={actions.inlineText}
			/>
			<WorkspaceUploadPreview
				uploads={files.workspaceUploads.uploads}
				onRemove={actions.removeWorkspaceUpload}
			/>
			{previewImage && (
				<ImageLightbox
					src={previewImage}
					onClose={() => setPreviewImage(null)}
				/>
			)}
			{previewText && (
				<TextPreviewDialog
					{...previewText}
					onClose={() => setPreviewText(null)}
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

	if (!state.speechSupported) {
		return null;
	}

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
				disabled={
					!state.speechRecording && (state.isReadOnly || state.isLoading)
				}
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

function Submit() {
	const { state } = useAgentComposer();

	return <SubmitButton label={state.isStreaming ? "Queue" : "Send"} />;
}

function SaveEdit() {
	return <SubmitButton label="Save Edit" />;
}

function SubmitButton({ label }: { label: string }) {
	const { state, actions, meta } = useAgentComposer();
	const tooltip = meta.sendShortcutLabel
		? `${label}: ${meta.sendShortcutLabel}`
		: label;

	if (!state.showSendButton) {
		return null;
	}

	let icon = <ArrowUpIcon />;

	if (state.isLoading && !state.isInterruptPending) {
		icon = <Spinner size="sm" loading aria-hidden="true" />;
	} else if (state.speechRecording) {
		icon = <CheckIcon />;
	}

	return (
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
					{icon}
					<span className="sr-only">
						{state.speechRecording ? "Accept voice input" : label}
					</span>
				</Button>
			</TooltipTrigger>
			<TooltipContent side="top">
				{state.speechRecording ? "Accept voice input" : tooltip}
			</TooltipContent>
		</Tooltip>
	);
}

function Stop() {
	const { state, actions } = useAgentComposer();

	if (!state.showStopButton) {
		return null;
	}

	return (
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
	);
}

function InterruptStatus() {
	const { state } = useAgentComposer();

	if (!state.isInterruptPending || !state.isStreaming) {
		return null;
	}

	return (
		<span role="status" className="sr-only">
			Interrupting. Waiting for the agent to stop.
		</span>
	);
}

function InvisibleCharacterWarning() {
	const { state } = useAgentComposer();
	const { invisibleCharCount } = state;

	if (invisibleCharCount === 0) {
		return null;
	}

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

	if (!state.warning) {
		return null;
	}

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
	Submit,
	SaveEdit,
	Stop,
	InterruptStatus,
	InvisibleCharacterWarning,
	Warning,
	EditBanner,
};
