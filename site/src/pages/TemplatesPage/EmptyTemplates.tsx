import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { TemplateExample } from "api/typesGenerated"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Stack } from "components/Stack/Stack"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { TemplateExampleCard } from "components/TemplateExampleCard/TemplateExampleCard"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link as RouterLink } from "react-router-dom"
import { Permissions } from "xServices/auth/authXService"

// Those are from https://github.com/coder/coder/tree/main/examples/templates
const featuredExampleIds = [
  "docker",
  "kubernetes",
  "aws-linux",
  "aws-windows",
  "gcp-linux",
  "gcp-windows",
]

const findFeaturedExamples = (examples: TemplateExample[]) => {
  const featuredExamples: TemplateExample[] = []

  // We loop the featuredExampleIds first to keep the order
  featuredExampleIds.forEach((exampleId) => {
    examples.forEach((example) => {
      if (exampleId === example.id) {
        featuredExamples.push(example)
      }
    })
  })

  return featuredExamples
}

export const EmptyTemplates: FC<{
  permissions: Permissions
  examples: TemplateExample[]
}> = ({ permissions, examples }) => {
  const styles = useStyles()
  const { t } = useTranslation("templatesPage")
  const featuredExamples = findFeaturedExamples(examples)

  if (permissions.createTemplates) {
    return (
      <TableEmpty
        message={t("empty.message")}
        description={
          <>
            You can create a template using our starter templates or{" "}
            <Link component={RouterLink} to="/new">
              uploading a template
            </Link>
            . You can also{" "}
            <Link
              href="https://coder.com/docs/coder-oss/latest/templates#add-a-template"
              target="_blank"
              rel="noreferrer"
            >
              use the CLI
            </Link>
            .
          </>
        }
        cta={
          <Stack alignItems="center" spacing={4}>
            <div className={styles.featuredExamples}>
              {featuredExamples.map((example) => (
                <TemplateExampleCard
                  example={example}
                  key={example.id}
                  className={styles.template}
                />
              ))}
            </div>

            <Button
              size="small"
              component={RouterLink}
              to="/starter-templates"
              className={styles.viewAllButton}
            >
              View all starter templates
            </Button>
          </Stack>
        }
      />
    )
  }

  return (
    <TableEmpty
      className={styles.withImage}
      message={t("empty.message")}
      description={t("empty.descriptionWithoutPermissions")}
      cta={<CodeExample code="coder templates init" />}
      image={
        <div className={styles.emptyImage}>
          <img src="/featured/templates.webp" alt="" />
        </div>
      }
    />
  )
}

const useStyles = makeStyles((theme) => ({
  withImage: {
    paddingBottom: 0,
  },

  emptyImage: {
    maxWidth: "50%",
    height: theme.spacing(40),
    overflow: "hidden",
    opacity: 0.85,

    "& img": {
      maxWidth: "100%",
    },
  },

  featuredExamples: {
    maxWidth: theme.spacing(100),
    display: "grid",
    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
    gap: theme.spacing(2),
    gridAutoRows: "min-content",
  },

  template: {
    backgroundColor: theme.palette.background.paperLight,

    "&:hover": {
      backgroundColor: theme.palette.divider,
    },
  },

  viewAllButton: {
    borderRadius: 9999,
  },
}))
