import { FC, PropsWithChildren } from "react";

const ReactMarkdown: FC<PropsWithChildren> = ({ children }) => {
  return <div data-testid="markdown">{children}</div>;
};

export default ReactMarkdown;
