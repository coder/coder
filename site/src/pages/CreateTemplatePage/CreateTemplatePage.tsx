import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { CreateTemplateForm } from "./CreateTemplateForm"

const CreateTemplatePage: FC = () => {
  const { t } = useTranslation("createTemplatePage")
  const navigate = useNavigate()

  const onCancel = () => {
    navigate(-1)
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
        <CreateTemplateForm isSubmitting={false} onCancel={onCancel} />
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTemplatePage
