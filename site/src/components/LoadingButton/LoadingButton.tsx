import { forwardRef } from "react";
import MuiLoadingButton, {
  LoadingButtonProps as MuiLoadingButtonProps,
} from "@mui/lab/LoadingButton";

export type LoadingButtonProps = MuiLoadingButtonProps;

export const LoadingButton = forwardRef<
  HTMLButtonElement,
  MuiLoadingButtonProps
>(({ children, loadingIndicator, ...buttonProps }, ref) => {
  return (
    <MuiLoadingButton
      variant="outlined"
      color="neutral"
      ref={ref}
      {...buttonProps}
    >
      {/* known issue: https://github.com/mui/material-ui/issues/27853 */}
      <span>
        {buttonProps.loading && loadingIndicator ? loadingIndicator : children}
      </span>
    </MuiLoadingButton>
  );
});
