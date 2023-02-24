import Button from "@material-ui/core/Button"
import Popover from "@material-ui/core/Popover"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { DropdownContent } from "components/DropdownButton/DropdownContent/DropdownContent"
import { FC, ReactNode, useRef, useState } from "react"
import { CancelButton } from "./ActionCtas"

export interface DropdownButtonProps {
  primaryAction: ReactNode
  secondaryActions: Array<{ action: string; button: ReactNode }>
  canCancel: boolean
  handleCancel?: () => void
}

export const DropdownButton: FC<DropdownButtonProps> = ({
  primaryAction,
  secondaryActions,
  canCancel,
  handleCancel,
}) => {
  const styles = useStyles()
  const theme = useTheme()
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "action-popover" : undefined
  const canOpen = secondaryActions.length > 0

  return (
    <span className={styles.buttonContainer}>
      {/* primary workspace CTA */}
      <span data-testid="primary-cta" className={styles.primaryCta}>
        {primaryAction}
      </span>
      {canCancel && handleCancel ? (
        <CancelButton handleAction={handleCancel} />
      ) : (
        <>
          {/* popover toggle button */}
          <Button
            variant="outlined"
            data-testid="workspace-actions-button"
            aria-controls="workspace-actions-menu"
            aria-haspopup="true"
            className={styles.dropdownButton}
            ref={anchorRef}
            disabled={!canOpen}
            onClick={() => {
              setIsOpen(true)
            }}
          >
            {isOpen ? (
              <CloseDropdown />
            ) : (
              <OpenDropdown
                color={canOpen ? undefined : theme.palette.action.disabled}
              />
            )}
          </Button>
          <Popover
            classes={{ paper: styles.popoverPaper }}
            id={id}
            open={isOpen}
            anchorEl={anchorRef.current}
            onClose={() => setIsOpen(false)}
            onBlur={() => setIsOpen(false)}
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
            <DropdownContent secondaryActions={secondaryActions} />
          </Popover>
        </>
      )}
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  buttonContainer: {
    display: "inline-flex",
  },
  dropdownButton: {
    borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
    minWidth: "unset",
    width: "64px", // matching cancel button so button grouping doesn't grow in size
    "& .MuiButton-label": {
      marginRight: "8px",
    },
  },
  primaryCta: {
    [theme.breakpoints.down("sm")]: {
      width: "100%",

      "& > *": {
        width: "100%",
      },
    },
  },
  popoverPaper: {
    padding: 0,
    width: theme.spacing(28),

    "& .MuiButton-root": {
      padding: theme.spacing(1, 2),
      borderRadius: 0,
      width: "100%",
      border: 0,

      "&:hover": {
        background: theme.palette.action.hover,
      },
    },
  },
}))
