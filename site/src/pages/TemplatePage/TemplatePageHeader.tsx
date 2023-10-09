import { type FC, useRef, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { useDeletionPopoverState } from "./useDeletionPopoverState";

import { useQuery } from "react-query";
import { workspacesByQuery } from "api/queries/workspaces";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
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
  const navigate = useNavigate();
  const popoverState = useDeletionPopoverState(template, onDeleteTemplate);

  const queryText = `template:${template.name}`;
  const workspaceCountQuery = useQuery({
    ...workspacesByQuery(queryText),
    select: (res) => res.count,
  });

  const hasIcon = template.icon && template.icon !== "";
  const safeToDeleteTemplate =
    workspaceCountQuery.status === "success" && workspaceCountQuery.data === 0;

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
                onDelete={popoverState.openDeleteConfirmation}
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

      {safeToDeleteTemplate ? (
        <DeleteDialog
          isOpen={popoverState.isDeleteDialogOpen}
          onConfirm={popoverState.confirmDelete}
          onCancel={popoverState.cancelDeleteConfirmation}
          entity="template"
          name={template.name}
        />
      ) : (
        <ConfirmDialog
          type="info"
          title="Unable to delete"
          hideCancel={false}
          open={popoverState.isDeleteDialogOpen}
          onClose={popoverState.cancelDeleteConfirmation}
          confirmText="See workspaces"
          confirmLoading={workspaceCountQuery.status !== "success"}
          onConfirm={() => {
            navigate({
              pathname: "/workspaces",
              search: new URLSearchParams({ filter: queryText }).toString(),
            });
          }}
          description={
            <>
              {workspaceCountQuery.isSuccess && (
                <>
                  This template is used by{" "}
                  <strong>
                    {workspaceCountQuery.data} workspace
                    {workspaceCountQuery.data === 1 ? "" : "s"}
                  </strong>
                  . Please delete all related workspaces before deleting this
                  template.
                </>
              )}

              {workspaceCountQuery.isLoading && (
                <>Loading information about workspaces used by this template.</>
              )}

              {workspaceCountQuery.isError && (
                <>Unable to determine workspaces used by this template.</>
              )}
            </>
          }
        />
      )}
    </Margins>
  );
};
