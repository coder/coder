import { Component, type ReactNode } from "react";
import { RuntimeErrorState } from "./RuntimeErrorState";

interface ErrorBoundaryProps {
  fallback?: ReactNode;
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Our app's Error Boundary
 * Read more about React Error Boundaries: https://react.dev/reference/react/Component#catching-rendering-errors-with-an-error-boundary
 */
export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  render(): ReactNode {
    if (this.state.error) {
      return (
        this.props.fallback ?? <RuntimeErrorState error={this.state.error} />
      );
    }

    return this.props.children;
  }
}
