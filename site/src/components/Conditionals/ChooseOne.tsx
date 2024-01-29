import {
  Children,
  type FC,
  type PropsWithChildren,
  type ReactNode,
} from "react";

export interface CondProps {
  condition?: boolean;
  children?: ReactNode;
}

/**
 * Wrapper component that attaches a condition to a child component so that ChooseOne can
 * determine which child to render. The last Cond in a ChooseOne is the fallback case and
 * should not have a condition.
 * @param condition boolean expression indicating whether the child should be rendered, or undefined
 * @returns child. Note that Cond alone does not enforce the condition; it should be used inside ChooseOne.
 */
export const Cond: FC<CondProps> = ({ children }) => {
  return <>{children}</>;
};

/**
 * Wrapper component for rendering exactly one of its children. Wrap each child in Cond to associate it
 * with a condition under which it should be rendered. If no conditions are met, the final child
 * will be rendered.
 * @returns one of its children, or null if there are no children
 * @throws an error if its last child has a condition prop, or any non-final children do not have a condition prop
 */
export const ChooseOne: FC<PropsWithChildren> = ({ children }) => {
  const childArray = Children.toArray(children) as JSX.Element[];
  if (childArray.length === 0) {
    return null;
  }
  const conditionedOptions = childArray.slice(0, childArray.length - 1);
  const defaultCase = childArray[childArray.length - 1];
  if (defaultCase.props.condition !== undefined) {
    throw new Error(
      "The last Cond in a ChooseOne was given a condition prop, but it is the default case.",
    );
  }
  if (conditionedOptions.some((cond) => cond.props.condition === undefined)) {
    throw new Error(
      "A non-final Cond in a ChooseOne does not have a condition prop or the prop is undefined.",
    );
  }
  const chosen = conditionedOptions.find((child) => child.props.condition);
  return chosen ?? defaultCase;
};
