import { Button, type ButtonProps } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";

type RetryButtonProps = Pick<ButtonProps, "aria-label" | "size" | "variant"> & {
	isRetrying: boolean;
	onRetry: () => void;
};

export const RetryButton: React.FC<RetryButtonProps> = ({
	isRetrying,
	onRetry,
	...buttonProps
}) => (
	<Button {...buttonProps} onClick={onRetry} disabled={isRetrying}>
		{isRetrying && <Spinner className="size-4" loading />}
		Retry
	</Button>
);
