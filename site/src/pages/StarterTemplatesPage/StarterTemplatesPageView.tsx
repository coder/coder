import { makeStyles } from "@material-ui/core/styles"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { StarterTemplatesContext } from "xServices/starterTemplates/starterTemplatesXService"

export interface StarterTemplatesPageViewProps {
  context: StarterTemplatesContext
}

export const StarterTemplatesPageView: FC<StarterTemplatesPageViewProps> = ({
  context,
}) => {
  const { t } = useTranslation("starterTemplatesPage")
  const styles = useStyles()

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>{t("title")}</PageHeaderTitle>
        <PageHeaderSubtitle>{t("subtitle")}</PageHeaderSubtitle>
      </PageHeader>

      <Maybe condition={Boolean(context.error)}>
        <AlertBanner error={context.error} severity="error" />
      </Maybe>

      <Maybe condition={Boolean(!context.starterTemplates)}>
        <Loader />
      </Maybe>

      <div className={styles.templates}>
        {context.starterTemplates &&
          context.starterTemplates.map((example) => (
            <Link to={example.id} className={styles.template} key={example.id}>
              <div className={styles.templateIcon}>
                <img src={example.icon} alt="" />
              </div>
              <div className={styles.templateInfo}>
                <span className={styles.templateName}>{example.name}</span>
                <span className={styles.templateDescription}>
                  {example.description}
                </span>
              </div>
            </Link>
          ))}
      </div>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  templates: {
    display: "grid",
    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
    gap: theme.spacing(2),
  },

  template: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    background: theme.palette.background.paper,
    textDecoration: "none",
    color: "inherit",
    display: "flex",
    alignItems: "center",

    "&:hover": {
      backgroundColor: theme.palette.background.paperLight,
    },
  },

  templateIcon: {
    width: theme.spacing(12),
    height: theme.spacing(12),
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& img": {
      height: theme.spacing(4),
    },
  },

  templateInfo: {
    padding: theme.spacing(2, 2, 2, 0),
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(0.5),
  },

  templateName: {
    fontSize: theme.spacing(2),
  },

  templateDescription: {
    fontSize: theme.spacing(1.75),
    color: theme.palette.text.secondary,
  },
}))
