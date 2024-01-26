import { type FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { useDeletionDialogState } from "./useDeletionDialogState";

import { useQuery } from "react-query";
import { workspaces } from "api/queries/workspaces";
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
import AddIcon from "@mui/icons-material/AddOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import EditIcon from "@mui/icons-material/EditOutlined";
import CopyIcon from "@mui/icons-material/FileCopyOutlined";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import Divider from "@mui/material/Divider";
import { Pill } from "components/Pill/Pill";

type TemplateMenuProps = {
  templateName: string;
  templateVersion: string;
  templateId: string;
  onDelete: () => void;
};

const TemplateMenu: FC<TemplateMenuProps> = ({
  templateName,
  templateVersion,
  templateId,
  onDelete,
}) => {
  const dialogState = useDeletionDialogState(templateId, onDelete);
  const navigate = useNavigate();
  const queryText = `template:${templateName}`;
  const workspaceCountQuery = useQuery({
    ...workspaces({ q: queryText }),
    select: (res) => res.count,
  });
  const safeToDeleteTemplate = workspaceCountQuery.data === 0;

  return (
    <>
      <MoreMenu>
        <MoreMenuTrigger>
          <ThreeDotsButton />
        </MoreMenuTrigger>
        <MoreMenuContent>
          <MoreMenuItem
            onClick={() => {
              navigate(`/templates/${templateName}/settings`);
            }}
          >
            <SettingsIcon />
            Settings
          </MoreMenuItem>

          <MoreMenuItem
            onClick={() => {
              navigate(
                `/templates/${templateName}/versions/${templateVersion}/edit`,
              );
            }}
          >
            <EditIcon />
            Edit files
          </MoreMenuItem>

          <MoreMenuItem
            onClick={() => {
              navigate(`/templates/new?fromTemplate=${templateName}`);
            }}
          >
            <CopyIcon />
            Duplicate&hellip;
          </MoreMenuItem>
          <Divider />
          <MoreMenuItem onClick={dialogState.openDeleteConfirmation} danger>
            <DeleteIcon />
            Delete&hellip;
          </MoreMenuItem>
        </MoreMenuContent>
      </MoreMenu>

      {safeToDeleteTemplate ? (
        <DeleteDialog
          isOpen={dialogState.isDeleteDialogOpen}
          onConfirm={dialogState.confirmDelete}
          onCancel={dialogState.cancelDeleteConfirmation}
          entity="template"
          name={templateName}
        />
      ) : (
        <ConfirmDialog
          type="info"
          title="Unable to delete"
          hideCancel={false}
          open={dialogState.isDeleteDialogOpen}
          onClose={dialogState.cancelDeleteConfirmation}
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
    </>
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

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            {!template.deprecated && (
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                component={RouterLink}
                to={`/templates/${template.name}/workspace`}
              >
                Create Workspace
              </Button>
            )}

            {permissions.canUpdateTemplate && (
              <TemplateMenu
                templateVersion={activeVersion.name}
                templateName={template.name}
                templateId={template.id}
                onDelete={onDeleteTemplate}
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

          {template.deprecated && <Pill type="warning">Deprecated</Pill>}
        </Stack>
      </PageHeader>
    </Margins>
  );
};
