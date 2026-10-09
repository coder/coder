import type React from "react";
import { AgentComposer, type AgentComposerBindings } from "./AgentComposer";
import {
	AgentComposerContainer,
	AgentComposerContextIndicator,
	AgentComposerOptionsControl,
	type AgentComposerSetup,
	AgentComposerSetupNotice,
	needsAgentSetup,
} from "./AgentComposerLayout";
import { QueuedMessagesList } from "./QueuedMessagesList";

type AgentComposerConfiguration = {
	bindings: AgentComposerBindings;
	options: React.ComponentProps<typeof AgentComposerOptionsControl>;
	setup: AgentComposerSetup;
	editor?: Omit<
		React.ComponentProps<typeof AgentComposer.Editor>,
		"hasWorkspace"
	> & {
		hasWorkspace?: boolean;
	};
	fillWidth?: boolean;
};

/** Composes a new chat using its organization's controlled settings. */
export const NewAgentComposer = ({
	bindings,
	options,
	setup,
	editor,
	fillWidth,
}: AgentComposerConfiguration) => {
	const showSetupNotice = needsAgentSetup(setup);

	return (
		<AgentComposer.Provider bindings={bindings}>
			<AgentComposerContainer fillWidth={fillWidth}>
				{showSetupNotice && (
					<AgentComposerSetupNotice
						{...setup}
						organizationId={options.chatOrganizationId}
					/>
				)}
				<AgentComposer.Frame showSetupNotice={showSetupNotice}>
					<AgentComposer.Warning />
					<AgentComposer.Attachments />
					<AgentComposer.Editor hasWorkspace={false} {...editor} />
					<AgentComposer.InvisibleCharacterWarning />
					<AgentComposer.Toolbar>
						<AgentComposerOptionsControl
							{...options}
							showAgentSetupNotice={showSetupNotice}
						/>
						<div className="flex shrink-0 items-center gap-2">
							<AgentComposer.VoiceInput />
							<AgentComposer.PrimaryAction />
						</div>
					</AgentComposer.Toolbar>
				</AgentComposer.Frame>
			</AgentComposerContainer>
		</AgentComposer.Provider>
	);
};

type ChatComposerProps = AgentComposerConfiguration & {
	queue?: Omit<React.ComponentProps<typeof QueuedMessagesList>, "className">;
	context?: React.ComponentProps<typeof AgentComposerContextIndicator>;
};

/** Composes an existing chat without taking ownership of its turn controller. */
export const ChatComposer = ({
	bindings,
	options,
	setup,
	editor,
	fillWidth,
	queue,
	context,
}: ChatComposerProps) => {
	const showSetupNotice = needsAgentSetup(setup);

	return (
		<AgentComposer.Provider bindings={bindings}>
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
						organizationId={options.chatOrganizationId}
					/>
				)}
				<AgentComposer.Frame showSetupNotice={showSetupNotice}>
					<AgentComposer.Warning />
					{bindings.isEditingHistoryMessage && <AgentComposer.EditBanner />}
					<AgentComposer.Attachments />
					<AgentComposer.Editor hasWorkspace={false} {...editor} />
					<AgentComposer.InvisibleCharacterWarning />
					<AgentComposer.Toolbar>
						<AgentComposerOptionsControl
							{...options}
							showAgentSetupNotice={showSetupNotice}
							hasContextUsage={context !== undefined}
						/>
						<div className="flex shrink-0 items-center gap-2">
							<AgentComposer.VoiceInput />
							{context && <AgentComposerContextIndicator {...context} />}
							<AgentComposer.PrimaryAction />
						</div>
					</AgentComposer.Toolbar>
				</AgentComposer.Frame>
			</AgentComposerContainer>
		</AgentComposer.Provider>
	);
};

/** Keeps the loading editor's draft handoff separate from loaded-chat behavior. */
export const LoadingChatComposer = ({
	bindings,
	options,
}: Pick<AgentComposerConfiguration, "bindings" | "options">) => (
	<AgentComposer.Provider bindings={bindings}>
		<AgentComposerContainer>
			<AgentComposer.Frame>
				<AgentComposer.Editor hasWorkspace={false} />
				<AgentComposer.InvisibleCharacterWarning />
				<AgentComposer.Toolbar>
					<AgentComposerOptionsControl {...options} />
					<div className="flex shrink-0 items-center gap-2">
						<AgentComposer.VoiceInput />
						<AgentComposer.PrimaryAction />
					</div>
				</AgentComposer.Toolbar>
			</AgentComposer.Frame>
		</AgentComposerContainer>
	</AgentComposer.Provider>
);
