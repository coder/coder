import React from "react"
import { RuntimeErrorState } from "../RuntimeErrorState/RuntimeErrorState"

type ErrorBoundaryProps = Record<string, unknown>

interface ErrorBoundaryState {
  error: Error | null
}

/**
 * Our app's Error Boundary
 * Read more about React Error Boundaries: https://reactjs.org/docs/error-boundaries.html
 */
export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): { error: Error } {
    return { error }
  }

  render(): React.ReactNode {
    if (this.state.error) {
      return <RuntimeErrorState error={this.state.error} />
    }

    return this.props.children
  }
}
