import { shallowEqual, useActor, useSelector } from "@xstate/react"
import {
  checkAuthorization,
  createWorkspace,
  getTemplates,
  getTemplateVersionSchema,
  getWorkspaceQuota,
} from "api/api"
import { FeatureNames } from "api/types"
import { ParameterSchema, Template, User, WorkspaceQuota } from "api/typesGenerated"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC, useContext, useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "xServices/StateContext"
import { CreateWorkspaceErrors, CreateWorkspacePageView } from "./CreateWorkspacePageView"

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId()
  const { template } = useParams()
  const templateName = template ? template : ""
  const navigate = useNavigate()
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const { me } = authState.context
  const featureVisibility = useSelector(
    xServices.entitlementsXService,
    selectFeatureVisibility,
    shallowEqual,
  )
  const workspaceQuotaEnabled = featureVisibility[FeatureNames.WorkspaceQuota]

  const [owner, setOwner] = useState<User | null>(me ?? null)
  const [templates, setTemplates] = useState<Template[]>()
  const [selectedTemplate, setSelectedTemplate] = useState<Template>()
  const [templateSchema, setTemplateSchema] = useState<ParameterSchema[]>()
  const [creatingWorkspace, setCreatingWorkspace] = useState<boolean>(false)
  const [createWorkspaceError, setCreateWorkspaceError] = useState<Error | unknown>()
  const [getTemplatesError, setGetTemplatesError] = useState<Error | unknown>()
  const [getTemplateSchemaError, setGetTemplateSchemaError] = useState<Error | unknown>()
  const [permissions, setPermissions] = useState<Record<string, boolean>>()
  const [checkPermissionsError, setCheckPermissionsError] = useState<Error | unknown>()
  const [workspaceQuota, setWorkspaceQuota] = useState<WorkspaceQuota>()
  const [getWorkspaceQuotaError, setGetWorkspaceQuotaError] = useState<Error | unknown>()

  useEffect(() => {
    setGetTemplatesError(undefined)
    getTemplates(organizationId).then(
      (res) => {
        const temps = res.filter((template) => template.name === templateName)
        const selectedTemps = res.length > 0 ? temps[0] : undefined
        setSelectedTemplate(selectedTemps)
        setTemplates(temps)
      },
      (err) => {
        setGetTemplatesError(err)
      },
    )
  }, [organizationId, templateName])

  useEffect(() => {
    if (!selectedTemplate) {
      return
    }

    setGetTemplateSchemaError(undefined)
    getTemplateVersionSchema(selectedTemplate.active_version_id).then(
      (res) => {
        // Only show parameters that are allowed to be overridden.
        // CLI code: https://github.com/coder/coder/blob/main/cli/create.go#L152-L155
        res = res.filter((param) => param.allow_override_source)
        setTemplateSchema(res)
      },
      (err) => {
        setGetTemplateSchemaError(err)
      },
    )
  }, [selectedTemplate])

  useEffect(() => {
    if (!organizationId) {
      return
    }

    // HACK: below, we pass in * for the owner_id, which is a hacky way of checking if the
    // current user can create a workspace on behalf of anyone within the org (only org owners should be able to do this).
    // This pattern should not be replicated outside of this narrow use case.
    const permissionsToCheck = {
      createWorkspaceForUser: {
        object: {
          resource_type: "workspace",
          organization_id: `${organizationId}`,
          owner_id: "*",
        },
        action: "create",
      },
    }

    setCheckPermissionsError(undefined)
    checkAuthorization({
      checks: permissionsToCheck,
    }).then(
      (res) => {
        setPermissions(res)
      },
      (err) => {
        setCheckPermissionsError(err)
      },
    )
  }, [organizationId])

  useEffect(() => {
    if (!workspaceQuotaEnabled) {
      // a limit of 0 will disable the component
      setWorkspaceQuota({
        user_workspace_count: 0,
        user_workspace_limit: 0,
      })
    }

    setGetWorkspaceQuotaError(undefined)
    getWorkspaceQuota(owner?.id ?? "me").then(
      (res) => {
        setWorkspaceQuota(res)
      },
      (err) => {
        setGetWorkspaceQuotaError(err)
      },
    )
  }, [owner?.id, workspaceQuotaEnabled])

  const hasErrors =
    getTemplatesError ||
    getTemplateSchemaError ||
    createWorkspaceError ||
    checkPermissionsError ||
    getWorkspaceQuotaError
      ? true
      : false

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        loadingTemplates={templates === undefined}
        loadingTemplateSchema={templateSchema === undefined}
        creatingWorkspace={creatingWorkspace}
        hasTemplateErrors={hasErrors}
        templateName={templateName}
        templates={templates}
        selectedTemplate={selectedTemplate}
        templateSchema={templateSchema}
        workspaceQuota={workspaceQuota}
        createWorkspaceErrors={{
          [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: getTemplatesError,
          [CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR]: getTemplateSchemaError,
          [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: createWorkspaceError,
          [CreateWorkspaceErrors.GET_WORKSPACE_QUOTA_ERROR]: getWorkspaceQuotaError,
          [CreateWorkspaceErrors.CHECK_PERMISSIONS_ERROR]: checkPermissionsError,
        }}
        canCreateForUser={permissions?.createWorkspaceForUser}
        owner={owner}
        setOwner={setOwner}
        onCancel={() => {
          navigate("/templates")
        }}
        onSubmit={(req) => {
          setCreatingWorkspace(true)

          createWorkspace(organizationId, owner?.id ?? "me", req).then(
            (res) => {
              navigate(`/@${res.owner_name}/${res.name}`)
            },
            (err) => {
              setCreateWorkspaceError(err)
            },
          )
        }}
      />
    </>
  )
}

export default CreateWorkspacePage
