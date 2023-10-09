import { type FC, useRef, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { useDeleteTemplate } from "./deleteTemplate";

import {
  AuthorizationResponse,
  Template,
  TemplateVersion,
} from "api/typesGenerated";

import { Avatar } from "components/Avatar/Avatar";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Stack } from "components/Stack/Stack";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";

import Button from "@mui/material/Button";
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import IconButton from "@mui/material/IconButton";
import AddIcon from "@mui/icons-material/AddOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import EditIcon from "@mui/icons-material/EditOutlined";
import CopyIcon from "@mui/icons-material/FileCopyOutlined";

type TemplateMenuProps = {
  templateName: string;
  templateVersion: string;
  onDelete: () => void;
  onMenuOpen?: () => void;
};

const TemplateMenu: FC<TemplateMenuProps> = ({
  templateName,
  templateVersion,
  onDelete,
}) => {
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
          <SettingsIcon />
          Settings
        </MenuItem>

        <MenuItem
          onClick={onMenuItemClick(() =>
            navigate(
              `/templates/${templateName}/versions/${templateVersion}/edit`,
            ),
          )}
        >
          <EditIcon />
          Edit files
        </MenuItem>

        <MenuItem
          onClick={onMenuItemClick(() =>
            navigate(`/templates/new?fromTemplate=${templateName}`),
          )}
        >
          <CopyIcon />
          Duplicate&hellip;
        </MenuItem>

        <MenuItem onClick={onMenuItemClick(onDelete)}>
          <DeleteIcon />
          Delete&hellip;
        </MenuItem>
      </Menu>
    </div>
  );
};

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
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              component={RouterLink}
              to={`/templates/${template.name}/workspace`}
            >
              Create Workspace
            </Button>

            {permissions.canUpdateTemplate && (
              <TemplateMenu
                templateVersion={activeVersion.name}
                templateName={template.name}
                onDelete={deleteTemplate.openDeleteConfirmation}
              />
            )}
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
