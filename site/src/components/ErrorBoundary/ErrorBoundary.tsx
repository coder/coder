import { Component, type ErrorInfo, type ReactNode } from "react";

interface ErrorBoundaryProps {
	/** Content to render when an error is caught. */
	fallback: ReactNode;
	children: ReactNode;
}

interface ErrorBoundaryState {
	hasError: boolean;
}

/**
 * A generic React error boundary that renders a fallback UI when a
 * child component throws during rendering.
 */
export class ErrorBoundary extends Component<
	ErrorBoundaryProps,
	ErrorBoundaryState
> {
	constructor(props: ErrorBoundaryProps) {
		super(props);
		this.state = { hasError: false };
	}

	static getDerivedStateFromError(): ErrorBoundaryState {
		return { hasError: true };
	}

	override componentDidCatch(error: Error, info: ErrorInfo): void {
		console.error("ErrorBoundary caught an error:", error, info);
	}

	override render(): ReactNode {
		if (this.state.hasError) {
			return this.props.fallback;
		}
		return this.props.children;
	}
}
