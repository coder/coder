import { FC } from "react"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { useTranslation } from "react-i18next"

export interface WorkspaceBuildParametersPageViewProps {
  isLoading: boolean
}

export const WorkspaceBuildParametersPageView: FC<
  WorkspaceBuildParametersPageViewProps
> = ({ isLoading }) => {
  const { t } = useTranslation("workspaceBuildParametersPage")

  return <FullPageForm title={t("title")}></FullPageForm>
}
