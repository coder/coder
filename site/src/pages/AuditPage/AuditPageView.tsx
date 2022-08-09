import { makeStyles } from "@material-ui/core/styles"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { AuditHelpTooltip } from "components/Tooltips"
import { FC } from "react"

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
  tooltipTitle: "Copy to clipboard and try the Coder CLI",
}

export const AuditPageView: FC = () => {
  const styles = useStyles()

  return (
    <Margins>
      <Stack justifyContent="space-between" className={styles.headingContainer}>
        <PageHeader className={styles.headingStyles}>
          <PageHeaderTitle>
            <Stack direction="row" spacing={1} alignItems="center">
              <span>{Language.title}</span>
              <AuditHelpTooltip />
            </Stack>
          </PageHeaderTitle>
          <PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
        </PageHeader>
        <CodeExample
          className={styles.codeExampleStyles}
          tooltipTitle={Language.tooltipTitle}
          code="coder audit [organization_ID]"
        />
      </Stack>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  headingContainer: {
    marginTop: theme.spacing(6),
    marginBottom: theme.spacing(5),
    flexDirection: "row",
    alignItems: "center",

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      alignItems: "start",
    },
  },
  headingStyles: {
    paddingTop: "0px",
    paddingBottom: "0px",
  },
  codeExampleStyles: {
    height: "fit-content",
  },
}))
