import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { StarterTemplatesContext } from "xServices/starterTemplates/starterTemplatesXService"

export interface StarterTemplatesPageViewProps {
  context: StarterTemplatesContext
}

export const StarterTemplatesPageView: FC<StarterTemplatesPageViewProps> = ({
  context,
}) => {
  const { t } = useTranslation("starterTemplatesPage")

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>{t("title")}</PageHeaderTitle>
        <PageHeaderSubtitle>{t("subtitle")}</PageHeaderSubtitle>
      </PageHeader>

      <Maybe condition={Boolean(context.error)}>
        <AlertBanner error={context.error} severity="error" />
      </Maybe>

      {context.starterTemplates &&
        context.starterTemplates.map((example) => (
          <div key={example.id}>{example.name}</div>
        ))}
    </Margins>
  )
}
