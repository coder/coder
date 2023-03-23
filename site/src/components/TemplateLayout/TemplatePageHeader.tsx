import Button from "@material-ui/core/Button"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import {
  AuthorizationResponse,
  Template,
  TemplateVersion,
} from "api/typesGenerated"
import { Avatar } from "components/Avatar/Avatar"
import { Maybe } from "components/Conditionals/Maybe"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { FC, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { useDeleteTemplate } from "./deleteTemplate"
import { Margins } from "components/Margins/Margins"
import MoreVertOutlined from "@material-ui/icons/MoreVertOutlined"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"

const Language = {
  variablesButton: "Variables",
  settingsButton: "Settings",
  createButton: "Create workspace",
  deleteButton: "Delete",
  editFilesButton: "Edit files",
}

const TemplateMenu: FC<{
  templateName: string
  templateVersion: string
  canEditFiles: boolean
  onDelete: () => void
}> = ({ templateName, templateVersion, canEditFiles, onDelete }) => {
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null)

  const handleClose = () => {
    setAnchorEl(null)
  }

  return (
    <div>
      <Button
        variant="outlined"
        aria-controls="template-options"
        aria-haspopup="true"
        onClick={(e) => setAnchorEl(e.currentTarget)}
      >
        <MoreVertOutlined />
      </Button>

      <Menu
        id="template-options"
        anchorEl={anchorEl}
        keepMounted
        open={Boolean(anchorEl)}
        onClose={handleClose}
      >
        <MenuItem
          onClick={handleClose}
          component={RouterLink}
          to={`/templates/${templateName}/settings`}
        >
          {Language.settingsButton}
        </MenuItem>
        {canEditFiles && (
          <MenuItem
            component={RouterLink}
            to={`/templates/${templateName}/versions/${templateVersion}/edit`}
            onClick={handleClose}
          >
            {Language.editFilesButton}
          </MenuItem>
        )}
        <MenuItem
          onClick={() => {
            onDelete()
            handleClose()
          }}
        >
          {Language.deleteButton}
        </MenuItem>
      </Menu>
    </div>
  )
}

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

export type TemplatePageHeaderProps = {
  template: Template
  activeVersion: TemplateVersion
  permissions: AuthorizationResponse
  canEditFiles: boolean
  onDeleteTemplate: () => void
}

export const TemplatePageHeader: FC<TemplatePageHeaderProps> = ({
  template,
  activeVersion,
  permissions,
  canEditFiles,
  onDeleteTemplate,
}) => {
  const hasIcon = template.icon && template.icon !== ""
  const deleteTemplate = useDeleteTemplate(template, onDeleteTemplate)

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            <CreateWorkspaceButton templateName={template.name} />
            <Maybe condition={permissions.canUpdateTemplate}>
              <TemplateMenu
                templateVersion={activeVersion.name}
                templateName={template.name}
                onDelete={deleteTemplate.openDeleteConfirmation}
                canEditFiles={canEditFiles}
              />
            </Maybe>
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
