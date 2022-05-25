import Button from "@material-ui/core/Button"
import React from "react"

export interface WorkspaceActionButtonProps {
  label: string
  icon: JSX.Element
  onClick: () => void
  className?: string
}

export const WorkspaceActionButton: React.FC<WorkspaceActionButtonProps> = ({ label, icon, onClick, className }) => {
  return (
    <Button className={className} startIcon={icon} onClick={onClick}>
      {label}
    </Button>
  )
}
