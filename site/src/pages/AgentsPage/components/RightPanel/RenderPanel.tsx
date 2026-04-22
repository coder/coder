import type { Spec } from "@json-render/react";
import { Renderer, StateProvider } from "@json-render/react";
import { LayoutDashboardIcon } from "lucide-react";
import { Component, type ErrorInfo, type FC, type ReactNode } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import { renderRegistry } from "../ChatElements/tools/renderCatalog";

interface RenderPanelProps {
	spec: Spec | null;
	title: string;
	isLoading?: boolean;
}

/**
 * Error boundary that catches rendering failures in the
 * JSON spec renderer and shows a graceful fallback.
 */
class RenderErrorBoundary extends Component<
	{ children: ReactNode },
	{ error: Error | null }
> {
	state: { error: Error | null } = { error: null };

	static getDerivedStateFromError(error: Error) {
		return { error };
	}

	componentDidCatch(error: Error, info: ErrorInfo) {
		console.error("RenderPanel: renderer error", error, info);
	}

	render() {
		if (this.state.error) {
			return (
				<div className="flex flex-col items-center justify-center gap-2 p-6 text-content-secondary">
					<span className="text-sm font-medium">Failed to render view</span>
					<span className="text-xs">{this.state.error.message}</span>
				</div>
			);
		}
		return this.props.children;
	}
}

export const RenderPanel: FC<RenderPanelProps> = ({
	spec,
	title,
	isLoading,
}) => {
	if (!spec && isLoading) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="h-6 w-6" />
				<span className="text-sm">Loading view…</span>
			</div>
		);
	}

	if (!spec) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<LayoutDashboardIcon className="h-6 w-6" />
				<span className="text-sm">No content</span>
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col overflow-hidden">
			{title && (
				<div className="shrink-0 border-b border-solid border-border-default px-4 py-2">
					<h3 className="truncate text-sm font-medium text-content-primary">
						{title}
					</h3>
				</div>
			)}
			<div className="min-h-0 flex-1 overflow-auto p-4">
				<RenderErrorBoundary>
					<StateProvider initialState={spec.state ?? {}}>
						<Renderer
							spec={spec}
							registry={renderRegistry}
							loading={isLoading}
						/>
					</StateProvider>
				</RenderErrorBoundary>
			</div>
		</div>
	);
};
