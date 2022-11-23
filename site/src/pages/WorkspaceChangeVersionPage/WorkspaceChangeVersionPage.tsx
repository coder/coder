import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { useMachine } from "@xstate/react"
import { Template, TemplateVersion, Workspace } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import { FC } from "react"
import { useNavigate, useParams } from "react-router-dom"
import { createDayString } from "util/createDayString"
import { changeWorkspaceVersionMachine } from "xServices/workspace/changeWorkspaceVersionXService"

const WorkspaceChangeVersionForm: FC<{
  isLoading: boolean
  workspace: Workspace
  template: Template
  versions: TemplateVersion[]
  onSubmit: (versionId: string) => void
  onCancel: () => void
}> = ({ isLoading, workspace, template, versions, onSubmit, onCancel }) => {
  const styles = useStyles()
  const formik = useFormik({
    initialValues: {
      versionId: workspace.latest_build.template_version_id,
    },
    onSubmit: ({ versionId }) => onSubmit(versionId),
  })

  return (
    <form onSubmit={formik.handleSubmit}>
      <Stack direction="column" spacing={3}>
        <Stack
          direction="row"
          spacing={2}
          className={styles.workspace}
          alignItems="center"
        >
          <div className={styles.workspaceIcon}>
            <img src={workspace.template_icon} alt="" />
          </div>
          <Stack direction="column" spacing={0.5}>
            <span className={styles.workspaceName}>{workspace.name}</span>

            <span className={styles.workspaceDescription}>
              {workspace.template_display_name.length > 0
                ? workspace.template_display_name
                : workspace.template_name}
            </span>
          </Stack>
        </Stack>

        <TextField
          select
          label="Workspace version"
          variant="outlined"
          SelectProps={{
            renderValue: (versionId: unknown) => {
              const version = versions.find(
                (version) => version.id === versionId,
              )
              if (!version) {
                throw new Error(`${versionId} not found.`)
              }
              return <>{version.name}</>
            },
          }}
          {...formik.getFieldProps("versionId")}
        >
          {versions
            .slice()
            .reverse()
            .map((version) => (
              <MenuItem
                key={version.id}
                value={version.id}
                className={styles.menuItem}
              >
                <div>
                  <div>{version.name}</div>
                  <div className={styles.versionDescription}>
                    Created by {version.created_by.username}{" "}
                    {createDayString(version.created_at)}
                  </div>
                </div>

                {template.active_version_id === version.id && (
                  <Pill
                    type="success"
                    text="Active"
                    className={styles.activePill}
                  />
                )}
              </MenuItem>
            ))}
        </TextField>
      </Stack>

      <FormFooter
        onCancel={onCancel}
        isLoading={isLoading}
        submitLabel="Update version"
      />
    </form>
  )
}

export const WorkspaceChangeVersionPage: FC = () => {
  const navigate = useNavigate()
  const { username: owner, workspace: workspaceName } = useParams() as {
    username: string
    workspace: string
  }
  const [state, send] = useMachine(changeWorkspaceVersionMachine, {
    context: {
      owner,
      workspaceName,
    },
    actions: {
      onUpdateVersion: () => {
        navigate(-1)
      },
    },
  })
  const { workspace, templateVersions, template } = state.context

  return (
    <FullPageForm title="Change version" onCancel={() => navigate(-1)}>
      {workspace && template && templateVersions ? (
        <WorkspaceChangeVersionForm
          isLoading={state.matches("updatingVersion")}
          versions={templateVersions}
          workspace={workspace}
          template={template}
          onSubmit={(versionId) => {
            send({
              type: "UPDATE_VERSION",
              versionId,
            })
          }}
          onCancel={() => {
            navigate(-1)
          }}
        />
      ) : (
        <Loader />
      )}
    </FullPageForm>
  )
}

const useStyles = makeStyles((theme) => ({
  workspace: {
    padding: theme.spacing(2.5, 3),
    borderRadius: theme.shape.borderRadius,
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
  },

  workspaceName: {
    fontSize: 16,
  },

  workspaceDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
  },

  workspaceIcon: {
    width: theme.spacing(5),
    lineHeight: 1,

    "& img": {
      width: "100%",
    },
  },

  menuItem: {
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
    position: "relative",
  },

  versionDescription: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },

  activePill: {
    position: "absolute",
    top: theme.spacing(2),
    right: theme.spacing(2),
  },
}))

export default WorkspaceChangeVersionPage
