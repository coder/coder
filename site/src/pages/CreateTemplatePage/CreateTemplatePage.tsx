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
  const [state, send] = useMachine(createTemplateMachine, {
    context: {
      organizationId,
      exampleId: searchParams.get("exampleId"),
    },
    actions: {
      onCreate: (_, { data }) => {
        navigate(`/templates/${data.name}`)
      },
    },
  })
  const { starterTemplate } = state.context
  const shouldDisplayForm =
    state.matches("idle") ||
    state.matches("creating") ||
    state.matches("created")

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
        {shouldDisplayForm && (
          <CreateTemplateForm
            starterTemplate={starterTemplate}
            isSubmitting={state.matches("creating")}
            onCancel={onCancel}
            onSubmit={(data) => {
              send({
                type: "CREATE",
                data,
              })
            }}
          />
        )}
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTemplatePage
