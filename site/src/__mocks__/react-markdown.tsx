import { FC } from "react"

const ReactMarkdown: FC<React.PropsWithChildren<unknown>> = ({ children }) => {
  return <div data-testid="markdown">{children}</div>
}

export default ReactMarkdown
