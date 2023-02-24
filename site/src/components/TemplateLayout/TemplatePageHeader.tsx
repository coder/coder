import Button from "@material-ui/core/Button"
import DeleteOutlined from "@material-ui/icons/DeleteOutlined"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import { AuthorizationResponse, Template } from "api/typesGenerated"
import { Avatar } from "components/Avatar/Avatar"
import { Maybe } from "components/Conditionals/Maybe"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { useDeleteTemplate } from "./deleteTemplate"
import { Margins } from "components/Margins/Margins"

const Language = {
  editButton: "Edit",
  settingsButton: "Settings",
  createButton: "Create workspace",
  deleteButton: "Delete",
}

const TemplateSettingsButton: FC<{ templateName: string }> = ({
  templateName,
}) => (
  <Button
    variant="outlined"
    component={RouterLink}
    to={`/templates/${templateName}/settings`}
    startIcon={<SettingsOutlined />}
  >
    {Language.settingsButton}
  </Button>
)

const CreateWorkspaceButton: FC<{
  templateName: string
  className?: string
}> = ({ templateName }) => (
  <Button
    startIcon={<AddCircleOutline />}
    component={RouterLink}
    to={`/templates/${templateName}/workspace`}
  >
    {Language.createButton}
  </Button>
)

const DeleteTemplateButton: FC<{ onClick: () => void }> = ({ onClick }) => (
  <Button variant="outlined" startIcon={<DeleteOutlined />} onClick={onClick}>
    {Language.deleteButton}
  </Button>
)

export type TemplatePageHeaderProps = {
  template: Template
  permissions: AuthorizationResponse
  onDeleteTemplate: () => void
}

export const TemplatePageHeader: FC<TemplatePageHeaderProps> = ({
  template,
  permissions,
  onDeleteTemplate,
}) => {
  const hasIcon = template.icon && template.icon !== ""
  const deleteTemplate = useDeleteTemplate(template, onDeleteTemplate)

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            <Maybe condition={permissions.canUpdateTemplate}>
              <DeleteTemplateButton
                onClick={deleteTemplate.openDeleteConfirmation}
              />
              <TemplateSettingsButton templateName={template.name} />
            </Maybe>
            <CreateWorkspaceButton templateName={template.name} />
          </>
        }
      >
        <Stack direction="row" spacing={3} alignItems="center">
          {hasIcon ? (
            <Avatar size="xl" src={template.icon} variant="square" fitImage />
          ) : (
            <Avatar size="xl">{template.name}</Avatar>
          )}

          <div>
            <PageHeaderTitle>
              {template.display_name.length > 0
                ? template.display_name
                : template.name}
            </PageHeaderTitle>
            {template.description !== "" && (
              <PageHeaderSubtitle condensed>
                {template.description}
              </PageHeaderSubtitle>
            )}
          </div>
        </Stack>
      </PageHeader>

      <DeleteDialog
        isOpen={deleteTemplate.isDeleteDialogOpen}
        confirmLoading={deleteTemplate.state.status === "deleting"}
        onConfirm={deleteTemplate.confirmDelete}
        onCancel={deleteTemplate.cancelDeleteConfirmation}
        entity="template"
        name={template.name}
      />
    </Margins>
  )
}
