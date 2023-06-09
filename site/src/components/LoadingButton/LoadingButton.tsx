import { FC } from "react"
import MuiLoadingButton, {
  LoadingButtonProps as MuiLoadingButtonProps,
} from "@mui/lab/LoadingButton"

export type LoadingButtonProps = MuiLoadingButtonProps

export const LoadingButton: FC<LoadingButtonProps> = ({
  children,
  loadingIndicator,
  ...buttonProps
}) => {
  return (
    <MuiLoadingButton variant="outlined" color="neutral" {...buttonProps}>
      {/* known issue: https://github.com/mui/material-ui/issues/27853 */}
      <span>
        {buttonProps.loading && loadingIndicator ? loadingIndicator : children}
      </span>
    </MuiLoadingButton>
  )
}
