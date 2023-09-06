import Button from "@mui/material/Button";
import AddIcon from "@mui/icons-material/AddOutlined";
import {
  AuthorizationResponse,
  Template,
  TemplateVersion,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Maybe } from "components/Conditionals/Maybe";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { FC, useRef, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { useDeleteTemplate } from "./deleteTemplate";
import { Margins } from "components/Margins/Margins";
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import DeleteOutlined from "@mui/icons-material/DeleteOutlined";
import EditOutlined from "@mui/icons-material/EditOutlined";
import FileCopyOutlined from "@mui/icons-material/FileCopyOutlined";
import IconButton from "@mui/material/IconButton";

const TemplateMenu: FC<{
  templateName: string;
  templateVersion: string;
  onDelete: () => void;
}> = ({ templateName, templateVersion, onDelete }) => {
  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const navigate = useNavigate();

  // Returns a function that will execute the action and close the menu
  const onMenuItemClick = (actionFn: () => void) => () => {
    setIsMenuOpen(false);

    actionFn();
  };

  return (
    <div>
      <IconButton
        aria-controls="template-options"
        aria-haspopup="true"
        onClick={() => setIsMenuOpen(true)}
        ref={menuTriggerRef}
        arial-label="More options"
      >
        <MoreVertOutlined />
      </IconButton>

      <Menu
        id="template-options"
        anchorEl={menuTriggerRef.current}
        open={isMenuOpen}
        onClose={() => setIsMenuOpen(false)}
      >
        <MenuItem
          onClick={onMenuItemClick(() =>
            navigate(`/templates/${templateName}/settings`),
          )}
        >
          <SettingsOutlined />
          Settings
        </MenuItem>
        <MenuItem
          onClick={onMenuItemClick(() =>
            navigate(`/templates/new?fromTemplate=${templateName}`),
          )}
        >
          <FileCopyOutlined />
          Duplicate
        </MenuItem>
        <MenuItem
          onClick={onMenuItemClick(() =>
            navigate(
              `/templates/${templateName}/versions/${templateVersion}/edit`,
            ),
          )}
        >
          <EditOutlined />
          Edit files
        </MenuItem>
        <MenuItem onClick={onMenuItemClick(onDelete)}>
          <DeleteOutlined />
          Delete
        </MenuItem>
      </Menu>
    </div>
  );
};

const CreateWorkspaceButton: FC<{
  templateName: string;
  className?: string;
}> = ({ templateName }) => (
  <Button
    variant="contained"
    startIcon={<AddIcon />}
    component={RouterLink}
    to={`/templates/${templateName}/workspace`}
  >
    Create Workspace
  </Button>
);

export type TemplatePageHeaderProps = {
  template: Template;
  activeVersion: TemplateVersion;
  permissions: AuthorizationResponse;
  onDeleteTemplate: () => void;
};

export const TemplatePageHeader: FC<TemplatePageHeaderProps> = ({
  template,
  activeVersion,
  permissions,
  onDeleteTemplate,
}) => {
  const hasIcon = template.icon && template.icon !== "";
  const deleteTemplate = useDeleteTemplate(template, onDeleteTemplate);

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
  );
};
