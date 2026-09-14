import type { FC } from "react";
import type { GitSSHKey } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { Spinner } from "#/components/Spinner/Spinner";

interface SSHKeysPageViewProps {
	isLoading: boolean;
	getSSHKeyError?: unknown;
	sshKey?: GitSSHKey;
	onRegenerateClick: () => void;
}

export const SSHKeysPageView: FC<SSHKeysPageViewProps> = ({
	isLoading,
	getSSHKeyError,
	sshKey,
	onRegenerateClick,
}) => {
	if (isLoading) {
		return (
			<div className="p-8">
				<Spinner size="lg" loading />
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			{/* Regenerating the key is not an option if getSSHKey fails.
        Only one of the error messages will exist at a single time */}
			{Boolean(getSSHKeyError) && <ErrorAlert error={getSSHKeyError} />}

			{sshKey && (
				<>
					<p className="m-0 text-sm text-content-secondary">
						The following public key is used to authenticate Git in workspaces.
						You may add it to Git services (such as GitHub) that you need to
						access from your workspace. Coder configures authentication via{" "}
						<code className="rounded-sm border border-border bg-surface-secondary px-1 py-0.5 text-xs text-content-primary">
							$GIT_SSH_COMMAND
						</code>
						.
					</p>
					<CodeExample secret={false} code={sshKey.public_key.trim()} />
					<div>
						<Button
							onClick={onRegenerateClick}
							data-testid="regenerate"
							variant="outline"
						>
							Regenerate&hellip;
						</Button>
					</div>
				</>
			)}
		</div>
	);
};
