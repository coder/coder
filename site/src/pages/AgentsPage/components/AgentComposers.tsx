import type React from "react";
import {
	AgentComposer,
	type AgentComposerBindings,
	AgentComposerProvider,
} from "./AgentComposer";
import {
	AgentComposerContainer,
	AgentComposerContextIndicator,
	type AgentComposerSetup,
	AgentComposerSetupNotice,
	needsAgentSetup,
} from "./AgentComposerLayout";
import { AgentComposerOptions } from "./AgentComposerOptions";
import { QueuedMessagesList } from "./QueuedMessagesList";

type AgentComposerConfiguration = {
	bindings: Omit<
		AgentComposerBindings,
		"queuedMessages" | "onPromoteQueuedMessage"
	>;
	model: React.ComponentProps<typeof AgentComposerOptions.Model>;
	tools: Omit<
		React.ComponentProps<typeof AgentComposerOptions.Provider>,
		"children"
	>;
	setup: AgentComposerSetup;
	editor?: Omit<
		React.ComponentProps<typeof AgentComposer.Editor>,
		"hasWorkspace"
	> & {
		hasWorkspace?: boolean;
	};
	fillWidth?: boolean;
};

type NewAgentComposerProps = Omit<
	AgentComposerConfiguration,
	"bindings" | "tools" | "editor"
> & {
	bindings: Omit<
		AgentComposerConfiguration["bindings"],
		| "isStreaming"
		| "onInterrupt"
		| "isInterruptPending"
		| "isEditingHistoryMessage"
		| "onCancelHistoryEdit"
		| "userPromptHistory"
	>;
	tools: Omit<AgentComposerConfiguration["tools"], "linkedWorkspace">;
	editor?: Omit<
		NonNullable<AgentComposerConfiguration["editor"]>,
		"hasWorkspace"
	>;
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
		<AgentComposerProvider bindings={bindings}>
			<AgentComposerOptions.Provider {...tools}>
				<AgentComposerContainer fillWidth={fillWidth}>
					{showSetupNotice && (
						<AgentComposerSetupNotice
							{...setup}
							organizationId={tools.organizationId}
						/>
					)}
					<AgentComposer.Frame showSetupNotice={showSetupNotice}>
						<AgentComposer.Warning />
						<AgentComposer.Attachments />
						<AgentComposer.Editor hasWorkspace={false} {...editor} />
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu
									showAgentSetupNotice={showSetupNotice}
								/>
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
		</AgentComposerProvider>
	);
};

type ChatComposerProps = AgentComposerConfiguration & {
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
		<AgentComposerProvider
			bindings={{
				...bindings,
				queuedMessages: queue?.messages,
				onPromoteQueuedMessage: queue?.onPromote,
			}}
		>
			<AgentComposerOptions.Provider {...tools}>
				<AgentComposerContainer
					fillWidth={fillWidth}
					isEditing={bindings.isEditingHistoryMessage}
				>
					{queue && queue.messages.length > 0 && (
						<QueuedMessagesList {...queue} className="mb-2" />
					)}
					{showSetupNotice && (
						<AgentComposerSetupNotice
							{...setup}
							organizationId={tools.organizationId}
						/>
					)}
					<AgentComposer.Frame showSetupNotice={showSetupNotice}>
						<AgentComposer.Warning />
						{bindings.isEditingHistoryMessage && <AgentComposer.EditBanner />}
						<AgentComposer.Attachments />
						<AgentComposer.Editor hasWorkspace={false} {...editor} />
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu
									showAgentSetupNotice={showSetupNotice}
								/>
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
									<HistoryEditComposerActions />
								) : (
									<ChatComposerActions />
								)}
							</div>
						</AgentComposer.Toolbar>
					</AgentComposer.Frame>
				</AgentComposerContainer>
			</AgentComposerOptions.Provider>
		</AgentComposerProvider>
	);
};

function ChatComposerActions() {
	return (
		<>
			<AgentComposer.Submit />
			<AgentComposer.Stop />
			<AgentComposer.InterruptStatus />
		</>
	);
}

function HistoryEditComposerActions() {
	return (
		<>
			<AgentComposer.SaveEdit />
			<AgentComposer.Stop />
			<AgentComposer.InterruptStatus />
		</>
	);
}

type LoadingChatComposerProps = {
	bindings: Pick<
		AgentComposerBindings,
		| "inputRef"
		| "initialValue"
		| "initialEditorState"
		| "remountKey"
		| "onContentChange"
		| "isDisabled"
	>;
	model: AgentComposerConfiguration["model"];
	tools: Pick<AgentComposerConfiguration["tools"], "planning">;
};

/** Composer that records draft changes while the chat loads. */
export const LoadingChatComposer = ({
	bindings,
	model,
	tools,
}: LoadingChatComposerProps) => (
	<AgentComposerProvider
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
					<AgentComposer.Editor hasWorkspace={false} />
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
	</AgentComposerProvider>
);
