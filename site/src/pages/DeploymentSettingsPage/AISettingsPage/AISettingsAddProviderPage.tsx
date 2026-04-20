import { ArrowLeft, ChevronDown } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Switch } from "#/components/Switch/Switch";

interface KeyPolicy {
	centralApiKey: boolean;
	allowUserKeys: boolean;
	centralKeyFallback: boolean;
}

const AISettingsAddProviderPage: FC = () => {
	const [keyPolicy, setKeyPolicy] = useState<KeyPolicy>({
		centralApiKey: true,
		allowUserKeys: true,
		centralKeyFallback: true,
	});

	return (
		<div>
			<Link
				to="/deployment/ai-settings/providers"
				className="inline-flex items-center gap-1 text-sm text-content-secondary no-underline hover:text-content-primary mb-6"
			>
				<ArrowLeft className="size-4" />
				Back to providers
			</Link>

			<h1 className="text-3xl font-semibold mt-0 mb-2">Add a provider</h1>
			<p className="text-content-secondary text-sm mt-0 mb-8 max-w-2xl">
				Connect third-party LLM services like OpenAI, Anthropic, or Google. Each
				provider supplies models that users can select for their conversations.
			</p>

			<div className="border border-solid border-border rounded-lg p-8 max-w-2xl">
				{/* API key */}
				<div className="mb-6">
					<label
						htmlFor="provider-api-key"
						className="block text-sm font-semibold text-content-primary mb-1"
					>
						API key
					</label>
					<p className="text-sm text-content-secondary mt-0 mb-2">
						Secret key used to authenticate requests to this provider.
					</p>
					<Input id="provider-api-key" type="password" />
				</div>

				{/* Base URL */}
				<div className="mb-6">
					<label
						htmlFor="provider-base-url"
						className="block text-sm font-semibold text-content-primary mb-1"
					>
						Base URL
					</label>
					<p className="text-sm text-content-secondary mt-0 mb-2">
						Custom endpoint for this provider. Leave empty to use the default.
					</p>
					<Input id="provider-base-url" type="url" />
				</div>

				{/* Icon */}
				<div className="mb-8">
					<span className="block text-sm font-semibold text-content-primary mb-2">
						Icon
					</span>
					<div className="flex items-center gap-2">
						<div className="w-10 h-10 border border-solid border-border rounded-md" />
						<button
							type="button"
							className="flex items-center gap-1 px-3 py-2 text-sm bg-transparent border border-solid border-border rounded-md cursor-pointer text-content-primary hover:bg-surface-secondary"
						>
							Emoji
							<ChevronDown className="size-4 text-content-secondary" />
						</button>
					</div>
				</div>

				{/* Key policy */}
				<div>
					<h3 className="text-sm font-semibold text-content-primary mt-0 mb-1">
						Key policy
					</h3>
					<p className="text-sm text-content-secondary mt-0 mb-4">
						Control which credential sources this provider can use.
					</p>

					<div className="border border-solid border-border rounded-lg divide-y divide-border">
						<div className="flex items-center justify-between p-4">
							<div>
								<div className="text-sm font-medium text-content-primary">
									Central API key
								</div>
								<div className="text-sm text-content-secondary">
									Use a deployment-managed API key for this provider
								</div>
							</div>
							<Switch
								checked={keyPolicy.centralApiKey}
								onCheckedChange={(checked) =>
									setKeyPolicy((prev) => ({
										...prev,
										centralApiKey: checked,
									}))
								}
							/>
						</div>

						<div className="flex items-center justify-between p-4">
							<div>
								<div className="text-sm font-medium text-content-primary">
									Allow user API keys
								</div>
								<div className="text-sm text-content-secondary">
									Let users provide their own API keys for this provider
								</div>
							</div>
							<Switch
								checked={keyPolicy.allowUserKeys}
								onCheckedChange={(checked) =>
									setKeyPolicy((prev) => ({
										...prev,
										allowUserKeys: checked,
									}))
								}
							/>
						</div>

						<div className="flex items-center justify-between p-4">
							<div>
								<div className="text-sm font-medium text-content-primary">
									Use central key as fallback
								</div>
								<div className="text-sm text-content-secondary">
									When a user has not saved a personal key, fall back to the
									central API key
								</div>
							</div>
							<Switch
								checked={keyPolicy.centralKeyFallback}
								onCheckedChange={(checked) =>
									setKeyPolicy((prev) => ({
										...prev,
										centralKeyFallback: checked,
									}))
								}
							/>
						</div>
					</div>
				</div>
			</div>

			{/* Actions */}
			<div className="flex items-center justify-end gap-3 mt-6 max-w-2xl">
				<Button variant="outline" asChild>
					<Link to="/deployment/ai-settings/providers">Cancel</Link>
				</Button>
				<Button>Add Provider</Button>
			</div>
		</div>
	);
};

export default AISettingsAddProviderPage;
