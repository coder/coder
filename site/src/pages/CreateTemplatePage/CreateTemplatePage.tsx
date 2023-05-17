import { useMachine } from "@xstate/react"
import { isApiValidationError } from "api/errors"
import { Maybe } from "components/Conditionals/Maybe"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { Loader } from "components/Loader/Loader"
import { Stack } from "components/Stack/Stack"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useSearchParams } from "react-router-dom"
import { pageTitle } from "utils/page"
import { createTemplateMachine } from "xServices/createTemplate/createTemplateXService"
import { CreateTemplateForm } from "./CreateTemplateForm"
import { ErrorAlert } from "components/Alert/ErrorAlert"

const CreateTemplatePage: FC = () => {
  const { t } = useTranslation("createTemplatePage")
  const navigate = useNavigate()
  const organizationId = useOrganizationId()
  const [searchParams] = useSearchParams()
  const [state, send] = useMachine(createTemplateMachine, {
    context: {
      organizationId,
      exampleId: searchParams.get("exampleId"),
      templateNameToCopy: searchParams.get("fromTemplate"),
    },
    actions: {
      onCreate: (_, { data }) => {
        navigate(`/templates/${data.name}`)
      },
    },
  })

  const {
    starterTemplate,
    parameters,
    error,
    file,
    jobError,
    jobLogs,
    variables,
  } = state.context
  const shouldDisplayForm = !state.hasTag("loading")
  const { entitlements } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled

  const onCancel = () => {
    navigate(-1)
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
        <Maybe condition={state.hasTag("loading")}>
          <Loader />
        </Maybe>

        <Stack spacing={6}>
          <Maybe condition={Boolean(error && !isApiValidationError(error))}>
            <ErrorAlert error={error} />
          </Maybe>

          {shouldDisplayForm && (
            <CreateTemplateForm
              copiedTemplate={state.context.copiedTemplate}
              allowAdvancedScheduling={allowAdvancedScheduling}
              error={error}
              starterTemplate={starterTemplate}
              isSubmitting={state.hasTag("submitting")}
              variables={variables}
              parameters={parameters}
              onCancel={onCancel}
              onSubmit={(data) => {
                send({
                  type: "CREATE",
                  data,
                })
              }}
              upload={{
                file,
                isUploading: state.matches("uploading"),
                onRemove: () => {
                  send("REMOVE_FILE")
                },
                onUpload: (file) => {
                  send({ type: "UPLOAD_FILE", file })
                },
              }}
              jobError={jobError}
              logs={jobLogs}
            />
          )}
        </Stack>
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTemplatePage
