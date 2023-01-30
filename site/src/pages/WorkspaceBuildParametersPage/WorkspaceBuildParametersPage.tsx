import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { pageTitle } from "util/page"
import { WorkspaceBuildParametersPageView } from "./WorkspaceBuildParametersPageView"

export const WorkspaceBuildParametersPage: FC = () => {
  const { t } = useTranslation("workspaceBuildParametersPage")

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>
      <WorkspaceBuildParametersPageView
        isLoading={false}
      />
    </>
  )
}
