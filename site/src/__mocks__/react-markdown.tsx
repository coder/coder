import { FC } from "react"

const ReactMarkdown: FC = ({ children }) => {
  return <div data-testid="markdown">{children}</div>
}

export default ReactMarkdown
