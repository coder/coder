import Button from "@material-ui/core/Button/Button"
import Link from "@material-ui/core/Link/Link"
import RemoveOutlined from "@material-ui/icons/RemoveOutlined"
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
import { useDeleteTemplate } from "./delete"

const TemplateSettingsButton: FC<{ templateName: string }> = ({
  templateName,
}) => (
  <Link
    underline="none"
    component={RouterLink}
    to={`/templates/${templateName}/settings`}
  >
    <Button variant="outlined" startIcon={<SettingsOutlined />}>
      Settings
    </Button>
  </Link>
)

const CreateWorkspaceButton: FC<{
  templateName: string
  className?: string
}> = ({ templateName, className }) => (
  <Link
    underline="none"
    component={RouterLink}
    to={`/templates/${templateName}/workspace`}
  >
    <Button className={className ?? ""} startIcon={<AddCircleOutline />}>
      Create workspace
    </Button>
  </Link>
)

const DeleteTemplateButton: FC<{ onClick: () => void }> = ({ onClick }) => (
  <Button startIcon={<RemoveOutlined />} onClick={onClick}>
    Delete
  </Button>
)

export const TemplatePageHeader: FC<{
  template: Template
  permissions: AuthorizationResponse
}> = ({ template, permissions }) => {
  const hasIcon = template.icon && template.icon !== ""
  const deleteTemplate = useDeleteTemplate(template)

  return (
    <>
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
            <PageHeaderSubtitle condensed>
              {template.description === ""
                ? "No description"
                : template.description}
            </PageHeaderSubtitle>
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
    </>
  )
}
