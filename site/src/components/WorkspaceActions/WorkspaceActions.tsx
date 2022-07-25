import Button from "@material-ui/core/Button"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import { FC, ReactNode, useEffect, useMemo, useRef, useState } from "react"
import { Workspace } from "../../api/typesGenerated"
import { getWorkspaceStatus, WorkspaceStatus } from "../../util/workspace"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { CancelButton, DeleteButton, StartButton, StopButton, UpdateButton } from "./ActionCtas"
import { ButtonTypesEnum, WorkspaceStateActions, WorkspaceStateEnum } from "./constants"

/**
 * Jobs submitted while another job is in progress will be discarded,
 * so check whether workspace job status has reached completion (whether successful or not).
 */
const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
  ["started", "stopped", "deleted", "error", "canceled"].includes(workspaceStatus)

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

  const canBeUpdated = workspace.outdated && canAcceptJobs(workspaceStatus)

  // actions are the primary and secondary CTAs that appear in the workspace actions dropdown
  const actions = useMemo(() => {
    if (!canBeUpdated) {
      return WorkspaceStateActions[workspaceState]
    }

    // if an update is available, we make the update button the primary CTA
    // and move the former primary CTA to the secondary actions list
    const updatedActions = { ...WorkspaceStateActions[workspaceState] }
    updatedActions.secondary.unshift(updatedActions.primary)
    updatedActions.primary = ButtonTypesEnum.update

    return updatedActions
  }, [canBeUpdated, workspaceState])

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
    [ButtonTypesEnum.update]: <UpdateButton handleAction={handleUpdate} />,
    [ButtonTypesEnum.start]: <StartButton handleAction={handleStart} />,
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.delete]: <DeleteButton handleAction={handleDelete} />,
    [ButtonTypesEnum.cancel]: <CancelButton handleAction={handleCancel} />,
    [ButtonTypesEnum.canceling]: disabledButton,
    [ButtonTypesEnum.disabled]: disabledButton,
    [ButtonTypesEnum.queued]: disabledButton,
    [ButtonTypesEnum.error]: disabledButton,
    [ButtonTypesEnum.loading]: disabledButton,
  }

  return (
    <span className={styles.buttonContainer}>
      {/* primary workspace CTA */}
      <span data-testid="primary-cta" className={styles.primaryCta}>
        {buttonMapping[actions.primary]}
      </span>

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
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  buttonContainer: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: `${theme.shape.borderRadius}px`,
    display: "inline-flex",
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
  primaryCta: {
    [theme.breakpoints.down("md")]: {
      width: "100%",

      "& > *": {
        width: "100%",
      },
    },
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
