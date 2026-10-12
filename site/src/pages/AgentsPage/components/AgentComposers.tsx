import { AgentComposer, type ComposerEditorProps } from "./AgentComposer";
import type {
	ComposerDraftBindings,
	ComposerEditorBindings,
	ComposerFileBindings,
} from "./AgentComposer/context";
import {
	AgentComposerContainer,
	type AgentComposerSetup,
	AgentComposerSetupNotice,
	needsAgentSetup,
} from "./AgentComposerLayout";
import {
	type AgentComposerModelProps,
	AgentComposerOptions,
} from "./AgentComposerOptions";
import type { AgentComposerOptionsData } from "./AgentComposerOptionsContext";

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
