import Button from "@material-ui/core/Button"
import { FC } from "react"

export interface WorkspaceActionButtonProps {
  label?: string
  icon: JSX.Element
  onClick: () => void
  className?: string
  ariaLabel?: string
}

export const WorkspaceActionButton: FC<React.PropsWithChildren<WorkspaceActionButtonProps>> = ({
  label,
  icon,
  onClick,
  className,
  ariaLabel,
}) => {
  return (
    <Button className={className} startIcon={icon} onClick={onClick} aria-label={ariaLabel}>
      {!!label && label}
    </Button>
  )
}
