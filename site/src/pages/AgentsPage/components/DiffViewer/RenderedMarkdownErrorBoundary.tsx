import { Component, type ErrorInfo, type ReactNode } from "react";

interface RenderedMarkdownErrorBoundaryProps {
	children: ReactNode;
}

interface RenderedMarkdownErrorBoundaryState {
	hasError: boolean;
}

/**
 * Renders a notice in place of children that throw during render. The
 * Markdown parser runs synchronously in render and can throw on
 * pathological nesting.
 */
export class RenderedMarkdownErrorBoundary extends Component<
	RenderedMarkdownErrorBoundaryProps,
	RenderedMarkdownErrorBoundaryState
> {
	state: RenderedMarkdownErrorBoundaryState = { hasError: false };

	static getDerivedStateFromError(): RenderedMarkdownErrorBoundaryState {
		return { hasError: true };
	}

	componentDidCatch(error: unknown, info: ErrorInfo): void {
		console.error("Markdown preview failed to render", error, info);
	}

	render(): ReactNode {
		if (this.state.hasError) {
			return (
				<div role="alert" className="px-3 py-2 text-xs text-content-secondary">
					Preview unavailable. Switch back to the diff to read this file.
				</div>
			);
		}
		return this.props.children;
	}
}
