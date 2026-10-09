import type React from "react";
import { type AgentComposerBindings, ComposerContext } from "./context";
import { ComposerFilePreviews } from "./useComposerFiles";
import { useComposerRuntime } from "./useComposerRuntime";

/** Supplies the agent draft implementation independently of the composed UI. */
export function AgentComposerProvider({
	bindings,
	children,
}: {
	bindings: AgentComposerBindings;
	children: React.ReactNode;
}) {
	const { context, previews } = useComposerRuntime(bindings);

	return (
		<ComposerContext value={context}>
			{children}
			<ComposerFilePreviews {...previews} />
		</ComposerContext>
	);
}
