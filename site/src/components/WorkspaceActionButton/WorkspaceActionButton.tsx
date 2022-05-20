import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"

export interface WorkspaceActionButtonProps {
  label: string
  loadingLabel: string
  isLoading: boolean
  icon: JSX.Element
  onClick: () => void
  className?: string
}

export const WorkspaceActionButton: React.FC<WorkspaceActionButtonProps> = ({
  label,
  loadingLabel,
  isLoading,
  icon,
  onClick,
  className,
}) => {
  const styles = useStyles()

  return (
    <Button
      className={className}
      startIcon={isLoading ? <CircularProgress size={12} className={styles.spinner} /> : icon}
      onClick={onClick}
      disabled={isLoading}
    >
      {isLoading ? loadingLabel : label}
    </Button>
  )
}

const useStyles = makeStyles((theme) => ({
  spinner: {
    color: theme.palette.text.disabled,
    marginRight: theme.spacing(1),
  },
}))
