import { PropsWithChildren } from "react";

export interface MaybeProps {
  condition: boolean;
}

/**
 * Wrapper component for conditionally rendering a child component without using "curly brace mode."
 * @param condition boolean expression that determines whether the child will be rendered
 * @returns the child or null
 */
export const Maybe = ({
  children,
  condition,
}: PropsWithChildren<MaybeProps>): JSX.Element | null => {
  return condition ? <>{children}</> : null;
};
