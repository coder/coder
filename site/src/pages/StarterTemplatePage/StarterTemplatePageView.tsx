import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import { MemoizedMarkdown } from "components/Markdown/Markdown"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { FC } from "react"
import { StarterTemplateContext } from "xServices/starterTemplates/starterTemplateXService"
import EyeIcon from "@material-ui/icons/VisibilityOutlined"
import PlusIcon from "@material-ui/icons/AddOutlined"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"

export interface StarterTemplatePageViewProps {
  context: StarterTemplateContext
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
  context,
}) => {
  const styles = useStyles()
  const { starterTemplate } = context
  const { t } = useTranslation("starterTemplatePage")

  if (context.error) {
    return (
      <Margins>
        <AlertBanner error={context.error} severity="error" />
      </Margins>
    )
  }

  if (!starterTemplate) {
    return <Loader />
  }

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            <Button
              component="a"
              target="_blank"
              href={starterTemplate.url}
              rel="noreferrer"
              startIcon={<EyeIcon />}
            >
              {t("actions.viewSourceCode")}
            </Button>
            <Button startIcon={<PlusIcon />}>{t("actions.useTemplate")}</Button>
          </>
        }
      >
        <PageHeaderTitle>{starterTemplate.name}</PageHeaderTitle>
        <PageHeaderSubtitle>{starterTemplate.description}</PageHeaderSubtitle>
      </PageHeader>

      <div className={styles.markdownSection} id="readme">
        <div className={styles.markdownWrapper}>
          <MemoizedMarkdown>{starterTemplate.markdown}</MemoizedMarkdown>
        </div>
      </div>
    </Margins>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    markdownSection: {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: theme.shape.borderRadius,
    },

    markdownWrapper: {
      padding: theme.spacing(5, 5, 8),
      maxWidth: 800,
      margin: "auto",
    },
  }
})
