import Button from "@material-ui/core/Button"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import { FC, ReactNode, useEffect, useRef, useState } from "react"
import { Workspace } from "../../api/typesGenerated"
import { getWorkspaceStatus } from "../../util/workspace"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { CancelButton, DeleteButton, StartButton, StopButton, UpdateButton } from "./ActionCtas"
import { ButtonTypesEnum, WorkspaceStateActions, WorkspaceStateEnum } from "./constants"

export interface WorkspaceActionsProps {
  workspace: Workspace
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspace,
  handleStart,
  handleStop,
  handleDelete,
  handleUpdate,
  handleCancel,
}) => {
  const styles = useStyles()
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "action-popover" : undefined

  const workspaceStatus: keyof typeof WorkspaceStateEnum = getWorkspaceStatus(
    workspace.latest_build,
  )
  const workspaceState = WorkspaceStateEnum[workspaceStatus]
  const actions = WorkspaceStateActions[workspaceState]

  /**
   * Ensures we close the popover before calling any action handler
   */
  useEffect(() => {
    setIsOpen(false)
    return () => {
      setIsOpen(false)
    }
  }, [workspaceStatus])

  const disabledButton = (
    <Button disabled className={styles.actionButton}>
      {workspaceState}
    </Button>
  )

  type ButtonMapping = {
    [key in ButtonTypesEnum]: ReactNode
  }

  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    [ButtonTypesEnum.start]: <StartButton handleAction={handleStart} />,
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.delete]: <DeleteButton handleAction={handleDelete} />,
    [ButtonTypesEnum.update]: (
      <UpdateButton
        handleAction={handleUpdate}
        workspace={workspace}
        workspaceStatus={workspaceStatus}
      />
    ),
    [ButtonTypesEnum.cancel]: <CancelButton handleAction={handleCancel} />,
    [ButtonTypesEnum.canceling]: disabledButton,
    [ButtonTypesEnum.disabled]: disabledButton,
    [ButtonTypesEnum.queued]: disabledButton,
    [ButtonTypesEnum.error]: disabledButton,
    [ButtonTypesEnum.loading]: disabledButton,
  }

  return (
    <div className={styles.buttonContainer}>
      {/* primary workspace CTA */}
      <span data-testid="primary-cta">{buttonMapping[actions.primary]}</span>

      {/* popover toggle button */}
      <Button
        data-testid="workspace-actions-button"
        aria-controls="workspace-actions-menu"
        aria-haspopup="true"
        className={styles.dropdownButton}
        ref={anchorRef}
        disabled={!actions.secondary.length}
        onClick={() => {
          setIsOpen(true)
        }}
      >
        {isOpen ? <CloseDropdown /> : <OpenDropdown />}
      </Button>

      <Popover
        classes={{ paper: styles.popoverPaper }}
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={() => setIsOpen(false)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
      >
        {/* secondary workspace CTAs */}
        <span data-testid="secondary-ctas">
          {actions.secondary.map((action) => (
            <div key={action} className={styles.popoverActionButton}>
              {buttonMapping[action]}
            </div>
          ))}
        </span>
      </Popover>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  buttonContainer: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: `${theme.shape.borderRadius}px`,
  },
  dropdownButton: {
    border: "none",
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
    minWidth: "unset",
    width: "35px",
    "& .MuiButton-label": {
      marginRight: "8px",
    },
  },
  actionButton: {
    // Set fixed width for the action buttons so they will not change the size
    // during the transitions
    width: theme.spacing(16),
    border: "none",
    borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
  },
  popoverActionButton: {
    "& .MuiButtonBase-root": {
      backgroundColor: "unset",
      justifyContent: "start",
      padding: "0px",
    },
  },
  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(3)}px`,
  },
}))
