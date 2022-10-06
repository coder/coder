import Link from "@material-ui/core/Link"
import { makeStyles, Theme, useTheme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"
import ReactMarkdown from "react-markdown"
import SyntaxHighlighter from "react-syntax-highlighter"
import { dracula as dark } from "react-syntax-highlighter/dist/cjs/styles/hljs"
import gfm from "remark-gfm"

export interface MarkdownProps {
  children: string
}

export const Markdown: FC<{ children: string }> = ({ children }) => {
  const theme: Theme = useTheme()
  const styles = useStyles()

  return (
    <ReactMarkdown
      remarkPlugins={[gfm]}
      components={{
        a: ({ href, target, children }) => (
          <Link href={href} target={target}>
            {children}
          </Link>
        ),

        code: ({ node, inline, className, children, ...props }) => {
          const match = /language-(\w+)/.exec(className || "")
          return !inline && match ? (
            <SyntaxHighlighter
              // Custom style to match our main colors
              style={{
                ...dark,
                hljs: {
                  ...dark.hljs,
                  background: theme.palette.background.default,
                  borderRadius: theme.shape.borderRadius,
                  color: theme.palette.text.primary,
                },
              }}
              language={match[1]}
              PreTag="div"
              {...props}
            >
              {String(children).replace(/\n$/, "")}
            </SyntaxHighlighter>
          ) : (
            <code className={styles.codeWithoutLanguage} {...props}>
              {children}
            </code>
          )
        },

        table: ({ children }) => {
          return (
            <TableContainer>
              <Table>{children}</Table>
            </TableContainer>
          )
        },

        tr: ({ children }) => {
          return <TableRow>{children}</TableRow>
        },

        thead: ({ children }) => {
          return <TableHead>{children}</TableHead>
        },

        tbody: ({ children }) => {
          return <TableBody>{children}</TableBody>
        },

        td: ({ children }) => {
          return <TableCell>{children}</TableCell>
        },

        th: ({ children }) => {
          return <TableCell>{children}</TableCell>
        },
      }}
    >
      {children}
    </ReactMarkdown>
  )
}

const useStyles = makeStyles((theme) => ({
  codeWithoutLanguage: {
    overflowX: "auto",
    padding: "0.5em",
    background: theme.palette.background.default,
    borderRadius: theme.shape.borderRadius,
    color: theme.palette.text.primary,
  },
}))
