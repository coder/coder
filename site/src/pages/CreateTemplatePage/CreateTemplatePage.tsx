import { useMachine } from "@xstate/react"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { Loader } from "components/Loader/Loader"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useSearchParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { createTemplateMachine } from "xServices/createTemplate/createTemplateXService"
import { CreateTemplateForm } from "./CreateTemplateForm"

const CreateTemplatePage: FC = () => {
  const { t } = useTranslation("createTemplatePage")
  const navigate = useNavigate()
  const organizationId = useOrganizationId()
  const [searchParams] = useSearchParams()
  const [state] = useMachine(createTemplateMachine, {
    context: {
      organizationId,
      exampleId: searchParams.get("exampleId"),
    },
    actions: {
      onCreate: () => {
        console.log("CREATE!")
      },
    },
  })
  const { starterTemplate } = state.context

  const onCancel = () => {
    navigate(-1)
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
        {state.hasTag("loading") && <Loader />}
        {state.matches("idle") && (
          <CreateTemplateForm
            starterTemplate={starterTemplate}
            isSubmitting={false}
            onCancel={onCancel}
          />
        )}
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTemplatePage
