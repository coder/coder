import { FC, PropsWithChildren } from "react"

const ReactMarkdown: FC<PropsWithChildren<unknown>> = ({ children }) => {
  return <div data-testid="markdown">{children}</div>
}

export default ReactMarkdown
