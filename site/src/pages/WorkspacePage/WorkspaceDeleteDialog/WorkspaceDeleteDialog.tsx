import { Workspace, CreateWorkspaceBuildRequest } from "api/typesGenerated";
import { useId, useState, FormEvent } from "react";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { type Interpolation, type Theme } from "@emotion/react";
import { colors } from "theme/colors";
import TextField from "@mui/material/TextField";
import { docs } from "utils/docs";
import Link from "@mui/material/Link";
import Checkbox from "@mui/material/Checkbox";

const styles = {
  workspaceInfo: (theme) => ({
    display: "flex",
    justifyContent: "space-between",
    backgroundColor: colors.gray[14],
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    padding: 12,
    marginBottom: 20,
    lineHeight: "1.3em",

    "& .name": {
      fontSize: 18,
      fontWeight: 800,
      color: theme.palette.text.primary,
    },

    "& .label": {
      fontSize: 11,
      color: theme.palette.text.secondary,
    },

    "& .info": {
      fontSize: 14,
      fontWeight: 500,
      color: theme.palette.text.primary,
    },
  }),
  orphanContainer: () => ({
    marginTop: 24,
    display: "flex",
    backgroundColor: colors.orange[15],
    justifyContent: "space-between",
    border: `1px solid ${colors.orange[11]}`,
    borderRadius: 8,
    padding: 12,
    lineHeight: "18px",

    "& .option": {
      color: colors.orange[11],
      "&.Mui-checked": {
        color: colors.orange[11],
      },
    },

    "& .info": {
      fontSize: "14px",
      color: colors.orange[10],
      fontWeight: 500,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;

interface WorkspaceDeleteDialogProps {
  workspace: Workspace;
  canUpdateTemplate: boolean;
  isOpen: boolean;
  onCancel: () => void;
  onConfirm: (arg: CreateWorkspaceBuildRequest["orphan"]) => void;
  workspaceBuildDateStr: string;
}
export const WorkspaceDeleteDialog = (props: WorkspaceDeleteDialogProps) => {
  const {
    workspace,
    canUpdateTemplate,
    isOpen,
    onCancel,
    onConfirm,
    workspaceBuildDateStr,
  } = props;
  const hookId = useId();
  const [userConfirmationText, setUserConfirmationText] = useState("");
  const [orphanWorkspace, setOrphanWorkspace] =
    useState<CreateWorkspaceBuildRequest["orphan"]>(false);
  const [isFocused, setIsFocused] = useState(false);

  const deletionConfirmed = workspace.name === userConfirmationText;
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (deletionConfirmed) {
      onConfirm(orphanWorkspace);
    }
  };

  const hasError = !deletionConfirmed && userConfirmationText.length > 0;
  const displayErrorMessage = hasError && !isFocused;
  const inputColor = hasError ? "error" : "primary";

  return (
    <ConfirmDialog
      type="delete"
      hideCancel={false}
      open={isOpen}
      title="Delete Workspace"
      onConfirm={() => onConfirm(orphanWorkspace)}
      onClose={onCancel}
      disabled={!deletionConfirmed}
      description={
        <>
          <div css={styles.workspaceInfo}>
            <div>
              <p className="name">{workspace.name}</p>
              <p className="label">workspace</p>
            </div>
            <div>
              <p className="info">{workspaceBuildDateStr}</p>
              <p className="label">created</p>
            </div>
          </div>

          <p>Deleting this workspace is irreversible!</p>
          <p>
            Type &ldquo;<strong>{workspace.name}</strong>&ldquo; below to
            confirm:
          </p>

          <form onSubmit={onSubmit}>
            <TextField
              fullWidth
              autoFocus
              css={{ marginTop: 32 }}
              name="confirmation"
              autoComplete="off"
              id={`${hookId}-confirm`}
              placeholder={workspace.name}
              value={userConfirmationText}
              onChange={(event) => setUserConfirmationText(event.target.value)}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              label="Workspace name"
              color={inputColor}
              error={displayErrorMessage}
              helperText={
                displayErrorMessage &&
                `${userConfirmationText} does not match the name of this workspace`
              }
              InputProps={{ color: inputColor }}
              inputProps={{
                "data-testid": "delete-dialog-name-confirmation",
              }}
            />
            {canUpdateTemplate && (
              <div css={styles.orphanContainer}>
                <div css={{ flexDirection: "column" }}>
                  <Checkbox
                    id="orphan_resources"
                    size="small"
                    color="warning"
                    onChange={() => {
                      setOrphanWorkspace(!orphanWorkspace);
                    }}
                    className="option"
                    name="orphan_resources"
                    checked={orphanWorkspace}
                    data-testid="orphan-checkbox"
                  />
                </div>
                <div css={{ flexDirection: "column" }}>
                  <p className="info">Orphan resources</p>
                  <span css={{ fontSize: "11px" }}>
                    Skip resource cleanup. Resources such as volumes and virtual
                    machines will not be destroyed.&nbsp;
                    <Link
                      href={docs("/workspaces#workspace-resources")}
                      target="_blank"
                      rel="noreferrer"
                    >
                      Learn more...
                    </Link>
                  </span>
                </div>
              </div>
            )}
          </form>
        </>
      }
    />
  );
};
