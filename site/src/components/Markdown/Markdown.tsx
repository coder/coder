import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { FC, memo } from "react"
import ReactMarkdown from "react-markdown"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import gfm from "remark-gfm"
import { colors } from "theme/colors"
import { darcula } from "react-syntax-highlighter/dist/cjs/styles/prism"

export interface MarkdownProps {
  children: string
}

export const Markdown: FC<{ children: string }> = ({ children }) => {
  const styles = useStyles()

  return (
    <ReactMarkdown
      className={styles.markdown}
      remarkPlugins={[gfm]}
      components={{
        a: ({ href, target, children }) => (
          <Link href={href} target={target}>
            {children}
          </Link>
        ),

        pre: ({ node, children }) => {
          const firstChild = node.children[0]
          // When pre is wrapping a code, the SyntaxHighlighter is already going
          // to wrap it with a pre so we don't need it
          if (firstChild.type === "element" && firstChild.tagName === "code") {
            return <>{children}</>
          }
          return <pre>{children}</pre>
        },

        code: ({ node, inline, className, children, style, ...props }) => {
          const match = /language-(\w+)/.exec(className || "")

          return !inline && match ? (
            <SyntaxHighlighter
              style={darcula}
              // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
              language={match[1].toLowerCase() ?? "language-shell"}
              useInlineStyles={false}
              // Use inline styles does not work correctly
              // https://github.com/react-syntax-highlighter/react-syntax-highlighter/issues/329
              codeTagProps={{ style: {} }}
              {...props}
            >
              {String(children)}
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

export const MemoizedMarkdown = memo(Markdown)

const useStyles = makeStyles((theme) => ({
  markdown: {
    fontSize: 16,
    lineHeight: "24px",

    "& h1, & h2, & h3, & h4, & h5, & h6": {
      marginTop: theme.spacing(4),
      marginBottom: theme.spacing(2),
      lineHeight: "1.25",
    },

    "& p": {
      marginTop: 0,
      marginBottom: theme.spacing(2),
    },

    "& ul, & ol": {
      display: "flex",
      flexDirection: "column",
      gap: theme.spacing(1),
      marginBottom: theme.spacing(2),
    },

    "& li > ul, & li > ol": {
      marginTop: theme.spacing(2),
    },

    "& li > p": {
      marginBottom: 0,
    },

    "& .prismjs": {
      background: theme.palette.background.paperLight,
      borderRadius: theme.shape.borderRadius,
      padding: theme.spacing(2, 3),
      overflowX: "auto",

      "& code": {
        color: theme.palette.text.secondary,
      },

      "& .key, & .property, & .inserted, .keyword": {
        color: colors.turquoise[7],
      },

      "& .deleted": {
        color: theme.palette.error.light,
      },
    },
  },

  codeWithoutLanguage: {
    padding: theme.spacing(0.125, 0.5),
    background: theme.palette.divider,
    borderRadius: 4,
    color: theme.palette.text.primary,
    fontSize: 14,
  },
}))
