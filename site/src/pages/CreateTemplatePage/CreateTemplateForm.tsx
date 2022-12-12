import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { TemplateExample } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate"
import { FC } from "react"
import { nameValidator, getFormHelpers, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  displayName: Yup.string().optional(),
  description: Yup.string().optional(),
  icon: Yup.string().optional(),
  default_ttl_hours: Yup.number(),
  allow_user_cancel_workspace_jobs: Yup.boolean(),
})

interface FormValues {
  name: string
  display_name: string
  description: string
  icon: string
  default_ttl_hours: number
  allow_user_cancel_workspace_jobs: boolean
}

const defaultInitialValues: FormValues = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  allow_user_cancel_workspace_jobs: false,
}

interface CreateTemplateFormProps {
  initialValues?: typeof defaultInitialValues
  starterTemplate?: TemplateExample
  errors?: unknown
  isSubmitting: boolean
  onCancel: () => void
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  initialValues = defaultInitialValues,
  starterTemplate,
  errors,
  isSubmitting,
  onCancel,
}) => {
  const styles = useStyles()
  const formFooterStyles = useFormFooterStyles()
  const form = useFormik({
    initialValues,
    validationSchema,
    onSubmit: () => {
      console.log("Submit")
    },
  })
  const getFieldHelpers = getFormHelpers<FormValues>(form, errors)

  return (
    <form onSubmit={form.handleSubmit}>
      <Stack direction="column" spacing={10} className={styles.formSections}>
        {/* General info */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>General info</h2>
            <p className={styles.formSectionInfoDescription}>
              The name is used to identify the template on the URL and also API.
              It has to be unique across your organization.
            </p>
          </div>

          <Stack
            direction="column"
            spacing={1}
            className={styles.formSectionFields}
          >
            {starterTemplate && <SelectedTemplate template={starterTemplate} />}

            <TextField
              {...getFieldHelpers("name")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              autoFocus
              fullWidth
              label="Name"
              variant="outlined"
            />
          </Stack>
        </div>

        {/* Display info  */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>Display info</h2>
            <p className={styles.formSectionInfoDescription}>
              Set the name that you want to use to display your template, a
              helpful description and icon.
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
            <TextField
              {...getFieldHelpers("display_name")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label="Display name"
              variant="outlined"
            />

            <TextField
              {...getFieldHelpers("description")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              rows={5}
              multiline
              fullWidth
              label="Description"
              variant="outlined"
            />

            <TextField
              {...getFieldHelpers("icon")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label="Icon"
              variant="outlined"
            />
          </Stack>
        </div>

        {/* Schedule */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>Schedule</h2>
            <p className={styles.formSectionInfoDescription}>
              Define when a workspace create from this template is going to
              stop.
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
            <TextField
              {...getFieldHelpers("default_ttl_hours")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label="Auto-stop default"
              variant="outlined"
              type="number"
              helperText="Time in hours"
            />
          </Stack>
        </div>

        {/* Operations */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>Operations</h2>
            <p className={styles.formSectionInfoDescription}>
              Allow or not users to run specific actions on the workspace.
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
            <label htmlFor="allow_user_cancel_workspace_jobs">
              <Stack direction="row" spacing={1}>
                <Checkbox
                  color="primary"
                  id="allow_user_cancel_workspace_jobs"
                  name="allow_user_cancel_workspace_jobs"
                  disabled={isSubmitting}
                  checked={form.values.allow_user_cancel_workspace_jobs}
                  onChange={form.handleChange}
                />

                <Stack direction="column" spacing={0.5}>
                  <span className={styles.optionText}>
                    Allow users to cancel in-progress workspace jobs.
                  </span>
                  <span className={styles.optionHelperText}>
                    Not recommended
                  </span>
                </Stack>
              </Stack>
            </label>
          </Stack>
        </div>

        <FormFooter
          styles={formFooterStyles}
          onCancel={onCancel}
          isLoading={isSubmitting}
          submitLabel="Create template"
        />
      </Stack>
    </form>
  )
}

const useStyles = makeStyles((theme) => ({
  formSections: {
    [theme.breakpoints.down("sm")]: {
      gap: theme.spacing(8),
    },
  },

  formSection: {
    display: "flex",
    alignItems: "flex-start",
    gap: theme.spacing(15),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(2),
    },
  },

  formSectionInfo: {
    width: 312,
    flexShrink: 0,
    position: "sticky",
    top: theme.spacing(3),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      position: "initial",
    },
  },

  formSectionInfoTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
  },

  formSectionInfoDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  },

  formSectionFields: {
    width: "100%",
  },

  optionText: {
    fontSize: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  optionHelperText: {
    fontSize: theme.spacing(1.5),
    color: theme.palette.text.secondary,
  },
}))

const useFormFooterStyles = makeStyles((theme) => ({
  button: {
    minWidth: theme.spacing(23),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: theme.spacing(2),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(1),
    },
  },
}))
