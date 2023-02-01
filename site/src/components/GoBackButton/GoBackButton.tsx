import Button from "@material-ui/core/Button"

interface GoBackButtonProps {
  onClick: () => void
}

export const Language = {
  ariaLabel: "Go back",
}

export const GoBackButton: React.FC<
  React.PropsWithChildren<GoBackButtonProps>
> = ({ onClick }) => {
  return (
    <Button onClick={onClick} size="small" aria-label={Language.ariaLabel}>
      Go back
    </Button>
  )
}
