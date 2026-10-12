import { InfoIcon } from "lucide-react";
import type React from "react";
import { AgentComposer, type ComposerEditorProps } from "./AgentComposer";
import type {
	ComposerChatBindings,
	ComposerDraftBindings,
	ComposerEditorBindings,
	ComposerFileBindings,
} from "./AgentComposer/context";
import {
	AgentComposerContainer,
	AgentComposerContextIndicator,
	type AgentComposerSetup,
	AgentComposerSetupNotice,
	needsAgentSetup,
} from "./AgentComposerLayout";
import {
	type AgentComposerModelProps,
	AgentComposerOptions,
} from "./AgentComposerOptions";
import type { AgentComposerOptionsData } from "./AgentComposerOptionsContext";
import { QueuedMessagesList } from "./QueuedMessagesList";

type ComposerConfiguration = {
	model: AgentComposerModelProps;
	tools: AgentComposerOptionsData;
	setup: AgentComposerSetup;
	editor?: ComposerEditorProps;
	fillWidth?: boolean;
};

type NewAgentComposerProps = ComposerConfiguration & {
	bindings: ComposerDraftBindings & {
		files: ComposerFileBindings;
	};
};

/** Composer for creating a chat with the selected organization's settings. */
export const NewAgentComposer = ({
	bindings,
	model,
	tools,
	setup,
	editor,
	fillWidth,
}: NewAgentComposerProps) => {
	const showSetupNotice = needsAgentSetup(setup);

	return (
		<AgentComposer.RuntimeProvider
			bindings={bindings}
			needsSetup={showSetupNotice}
		>
			<AgentComposerOptions.Provider {...tools}>
				<AgentComposerContainer fillWidth={fillWidth}>
					{showSetupNotice && (
						<AgentComposerSetupNotice
							{...setup}
							organizationId={tools.organizationId}
						/>
					)}
					<AgentComposer.Frame>
						<AgentComposer.Warning />
						<AgentComposer.Attachments />
						<AgentComposer.Editor {...editor} />
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu />
								<AgentComposerOptions.Model {...model} />
								<AgentComposerOptions.PlanningBadge />
								<AgentComposerOptions.Badges />
							</AgentComposerOptions.Frame>
							<div className="flex shrink-0 items-center gap-2">
								<AgentComposer.VoiceInput />
								<AgentComposer.Submit />
							</div>
						</AgentComposer.Toolbar>
					</AgentComposer.Frame>
				</AgentComposerContainer>
			</AgentComposerOptions.Provider>
		</AgentComposer.RuntimeProvider>
	);
};

type ChatComposerProps = ComposerConfiguration & {
	bindings: ComposerDraftBindings &
		ComposerChatBindings & {
			files: ComposerFileBindings;
		};
	queue?: Omit<React.ComponentProps<typeof QueuedMessagesList>, "className">;
	context?: React.ComponentProps<typeof AgentComposerContextIndicator>;
	/**
	 * Replaces the editor with this notice and shows the settings without
	 * offering changes or sending.
	 */
	readOnlyNotice?: string;
};

/** Composer for sending, queuing, and editing messages in an existing chat. */
export const ChatComposer = ({
	bindings,
	model,
	tools,
	setup,
	editor,
	fillWidth,
	queue,
	context,
	readOnlyNotice,
}: ChatComposerProps) => {
	const showSetupNotice = needsAgentSetup(setup);
	const isViewOnly = readOnlyNotice !== undefined;
	// With a context indicator beside the send button, the planning badge
	// joins the measured badge row so it can overflow.
	const hasContextIndicator = context !== undefined;

	return (
		<AgentComposer.RuntimeProvider
			needsSetup={showSetupNotice}
			bindings={{
				...bindings,
				queuedMessages: queue?.messages,
				onPromoteQueuedMessage: queue?.onPromote,
			}}
		>
			<AgentComposerOptions.Provider {...tools} isViewOnly={isViewOnly}>
				<AgentComposerContainer fillWidth={fillWidth}>
					{queue && queue.messages.length > 0 && (
						<QueuedMessagesList {...queue} className="mb-2" />
					)}
					{showSetupNotice && (
						<AgentComposerSetupNotice
							{...setup}
							organizationId={tools.organizationId}
						/>
					)}
					<AgentComposer.Frame
						className={
							isViewOnly ? "border border-solid border-border" : undefined
						}
					>
						<AgentComposer.Warning />
						{bindings.isEditingHistoryMessage && <AgentComposer.EditBanner />}
						<AgentComposer.Attachments />
						{isViewOnly ? (
							<div
								role="status"
								className="flex items-center gap-2 px-3 py-2 text-[13px] text-content-primary"
							>
								<InfoIcon className="size-4 shrink-0 text-content-link" />
								{readOnlyNotice}
							</div>
						) : (
							// Commands act on the whole chat, not an edited history message.
							<AgentComposer.Editor
								{...editor}
								slashCommands={
									bindings.isEditingHistoryMessage
										? undefined
										: editor?.slashCommands
								}
							/>
						)}
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu />
								<AgentComposerOptions.Model {...model} />
								{!hasContextIndicator && <AgentComposerOptions.PlanningBadge />}
								<AgentComposerOptions.Badges
									includePlanning={hasContextIndicator}
								/>
							</AgentComposerOptions.Frame>
							<div className="flex shrink-0 items-center gap-2">
								{!isViewOnly && <AgentComposer.VoiceInput />}
								{context && <AgentComposerContextIndicator {...context} />}
								{!isViewOnly && <AgentComposer.Submit />}
								{!isViewOnly && <AgentComposer.Stop />}
								<AgentComposer.InterruptStatus />
							</div>
						</AgentComposer.Toolbar>
					</AgentComposer.Frame>
				</AgentComposerContainer>
			</AgentComposerOptions.Provider>
		</AgentComposer.RuntimeProvider>
	);
};

type LoadingChatComposerProps = {
	bindings: ComposerEditorBindings & {
		isDisabled: boolean;
		hasModelOptions: boolean;
	};
	model: AgentComposerModelProps;
	tools: { planning: AgentComposerOptionsData["planning"] };
};

// The chat cannot accept messages until it loads; the composer only
// records the draft.
const ignoreSend = () => {};

/** Composer that records draft changes while the chat loads. */
export const LoadingChatComposer = ({
	bindings,
	model,
	tools,
}: LoadingChatComposerProps) => (
	<AgentComposer.RuntimeProvider
		bindings={{ ...bindings, onSend: ignoreSend, files: { attachments: [] } }}
	>
		<AgentComposerOptions.Provider {...tools}>
			<AgentComposerContainer>
				<AgentComposer.Frame>
					<AgentComposer.Editor />
					<AgentComposer.InvisibleCharacterWarning />
					<AgentComposer.Toolbar>
						<AgentComposerOptions.Frame>
							<AgentComposerOptions.Menu />
							<AgentComposerOptions.Model {...model} />
							<AgentComposerOptions.PlanningBadge />
							{/* Keeps the empty badge row so the model pill sizes as in
							the loaded chat composer. */}
							<AgentComposerOptions.Badges />
						</AgentComposerOptions.Frame>
						<div className="flex shrink-0 items-center gap-2">
							<AgentComposer.VoiceInput />
							<AgentComposer.Submit />
						</div>
					</AgentComposer.Toolbar>
				</AgentComposer.Frame>
			</AgentComposerContainer>
		</AgentComposerOptions.Provider>
	</AgentComposer.RuntimeProvider>
);
