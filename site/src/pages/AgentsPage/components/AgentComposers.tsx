import type React from "react";
import {
	AgentComposer,
	AgentComposerRuntimeProvider,
	type ComposerChatBindings,
	type ComposerDraftBindings,
	type ComposerEditorBindings,
	type ComposerEditorProps,
	type ComposerFileBindings,
} from "./AgentComposer";
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
	bindings: ComposerDraftBindings & { files: ComposerFileBindings };
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
		<AgentComposerRuntimeProvider
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
		</AgentComposerRuntimeProvider>
	);
};

type ChatComposerProps = ComposerConfiguration & {
	bindings: ComposerDraftBindings &
		ComposerChatBindings & { files: ComposerFileBindings };
	queue?: Omit<React.ComponentProps<typeof QueuedMessagesList>, "className">;
	context?: React.ComponentProps<typeof AgentComposerContextIndicator>;
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
}: ChatComposerProps) => {
	const showSetupNotice = needsAgentSetup(setup);

	return (
		<AgentComposerRuntimeProvider
			needsSetup={showSetupNotice}
			bindings={{
				...bindings,
				queuedMessages: queue?.messages,
				onPromoteQueuedMessage: queue?.onPromote,
			}}
		>
			<AgentComposerOptions.Provider {...tools}>
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
					<AgentComposer.Frame>
						<AgentComposer.Warning />
						{bindings.isEditingHistoryMessage && <AgentComposer.EditBanner />}
						<AgentComposer.Attachments />
						{/* Commands act on the whole chat, not an edited history message. */}
						<AgentComposer.Editor
							{...editor}
							slashCommands={
								bindings.isEditingHistoryMessage
									? undefined
									: editor?.slashCommands
							}
						/>
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu />
								<AgentComposerOptions.Model {...model} />
								{context ? (
									<AgentComposerOptions.Badges
										leadingBadges={
											tools.planning.enabled ? [{ kind: "planning" }] : []
										}
									/>
								) : (
									<>
										<AgentComposerOptions.PlanningBadge />
										<AgentComposerOptions.Badges />
									</>
								)}
							</AgentComposerOptions.Frame>
							<div className="flex shrink-0 items-center gap-2">
								<AgentComposer.VoiceInput />
								{context && <AgentComposerContextIndicator {...context} />}
								{bindings.isEditingHistoryMessage ? (
									<AgentComposer.SaveEdit />
								) : (
									<AgentComposer.Submit />
								)}
								<AgentComposer.Stop />
								<AgentComposer.InterruptStatus />
							</div>
						</AgentComposer.Toolbar>
					</AgentComposer.Frame>
				</AgentComposerContainer>
			</AgentComposerOptions.Provider>
		</AgentComposerRuntimeProvider>
	);
};

type LoadingChatComposerProps = {
	bindings: ComposerEditorBindings & { isDisabled: boolean };
	model: AgentComposerModelProps;
	tools: { planning: ComposerConfiguration["tools"]["planning"] };
};

/** Composer that records draft changes while the chat loads. */
export const LoadingChatComposer = ({
	bindings,
	model,
	tools,
}: LoadingChatComposerProps) => (
	<AgentComposerRuntimeProvider
		bindings={{
			...bindings,
			onSend: () => {},
			isLoading: false,
			hasModelOptions: false,
		}}
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
						</AgentComposerOptions.Frame>
						<div className="flex shrink-0 items-center gap-2">
							<AgentComposer.VoiceInput />
							<AgentComposer.Submit />
						</div>
					</AgentComposer.Toolbar>
				</AgentComposer.Frame>
			</AgentComposerContainer>
		</AgentComposerOptions.Provider>
	</AgentComposerRuntimeProvider>
);
