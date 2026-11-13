import type { GitSSHKey } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";

type SSHKeysPageViewProps = {
	isLoading: boolean;
	getSSHKeyError?: unknown;
	sshKey?: GitSSHKey;
	onRegenerateClick: () => void;
	showSignCommits: boolean;
	signCommitsError?: unknown;
	signCommits: boolean;
	isSignCommitsDisabled: boolean;
	onSignCommitsChange: (checked: boolean) => void;
};

export const SSHKeysPageView: React.FC<SSHKeysPageViewProps> = ({
	isLoading,
	getSSHKeyError,
	sshKey,
	onRegenerateClick,
	showSignCommits,
	signCommitsError,
	signCommits,
	isSignCommitsDisabled,
	onSignCommitsChange,
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
						<code className="rounded-sm border bg-surface-secondary px-1 py-0.5 text-xs text-content-primary">
							$GIT_SSH_COMMAND
						</code>
						.
					</p>
					<CodeExample secret={false} code={sshKey.public_key.trim()} />
					{showSignCommits && (
						<>
							{Boolean(signCommitsError) && (
								<ErrorAlert error={signCommitsError} />
							)}
							<div className="flex items-start gap-2">
								<Checkbox
									id="sign-commits"
									checked={signCommits}
									disabled={isSignCommitsDisabled}
									onCheckedChange={(checked) =>
										onSignCommitsChange(checked === true)
									}
								/>
								<div className="flex flex-col gap-1">
									<Label
										htmlFor="sign-commits"
										className="text-content-primary"
									>
										Sign my commits with my Coder SSH key
									</Label>
									<p className="m-0 text-xs text-content-secondary">
										Signs every commit and tag made in your workspaces with the
										key above. Add the public key to GitHub or GitLab as a{" "}
										<strong>signing key</strong>; an authentication key does not
										verify commits.{" "}
										<Link
											href="https://github.com/settings/ssh/new"
											target="_blank"
											rel="noreferrer"
											size="sm"
										>
											Add it on GitHub
										</Link>
										. Requires git 2.34 or later in the workspace. Takes effect
										the next time your workspace agent connects, such as after a
										restart. This overrides any signing settings in your own git
										config.
									</p>
								</div>
							</div>
						</>
					)}
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
