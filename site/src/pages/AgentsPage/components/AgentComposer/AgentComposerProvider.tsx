import type React from "react";
import { type AgentComposerBindings, ComposerContext } from "./context";
import { ComposerFilePreviews } from "./useComposerFiles";
import { useComposerRuntime } from "./useComposerRuntime";

/** Provides draft interactions and attachment previews to composer children. */
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
