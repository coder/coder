import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { Template, TemplateVersion, Workspace } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import { FC } from "react"
import { createDayString } from "util/createDayString"
import * as Yup from "yup"

const validationSchema = Yup.object({
  versionId: Yup.string().required(),
})

export const WorkspaceChangeVersionForm: FC<{
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
    validationSchema,
    onSubmit: ({ versionId }) => onSubmit(versionId),
  })
  const autocompleteValue = versions.find(
    (version) => version.id === formik.values.versionId,
  )

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

        <Autocomplete
          disableClearable
          options={versions.slice().reverse()}
          value={autocompleteValue}
          onChange={async (_event, value) => {
            if (value) {
              await formik.setFieldValue("versionId", value.id)
            }
          }}
          renderInput={(params) => (
            <TextField
              {...params}
              {...formik.getFieldProps("versionId")}
              label="Workspace version"
              variant="outlined"
              fullWidth
            />
          )}
          getOptionLabel={(version: TemplateVersion) => version.name}
          renderOption={(version: TemplateVersion) => (
            <div className={styles.menuItem}>
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
            </div>
          )}
        />
      </Stack>

      <FormFooter
        onCancel={onCancel}
        isLoading={isLoading}
        submitLabel="Update version"
      />
    </form>
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
    paddingTop: theme.spacing(1),
    paddingBottom: theme.spacing(1),
    position: "relative",
    width: "100%",
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
