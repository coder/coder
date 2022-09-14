import { Children, PropsWithChildren } from "react"

export interface CondProps {
  condition: boolean
}

/**
 * Wrapper component that attaches a condition to a child component so that ChooseOne can
 * determine which child to render. The last Cond in a ChooseOne is the fallback case; set
 * its `condition` to `true` to avoid confusion.
 * @param condition boolean expression indicating whether the child should be rendered
 * @returns child. Note that Cond alone does not enforce the condition; it should be used inside ChooseOne.
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export const Cond = ({ children, condition }: PropsWithChildren<CondProps>): JSX.Element => {
  return <>{children}</>
}

/**
 * Wrapper component for rendering exactly one of its children. Wrap each child in Cond to associate it
 * with a condition under which it should be rendered. If no conditions are met, the final child
 * will be rendered.
 * @returns one of its children
 */
export const ChooseOne = ({ children }: PropsWithChildren): JSX.Element => {
  const childArray = Children.toArray(children) as JSX.Element[]
  const chosen = childArray.find((child) => child.props.condition)
  return chosen ?? childArray[childArray.length - 1]
}
