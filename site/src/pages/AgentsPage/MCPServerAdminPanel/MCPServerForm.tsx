import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ArrowLeftIcon, Trash2Icon } from "lucide-react";
import { type FC, useState } from "react";
import { SectionHeader } from "../SectionHeader";

const inputClassName =
	"w-full rounded-lg border border-border bg-surface-primary px-3 py-2 text-[13px] text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";

const labelClassName =
	"block text-[13px] font-medium text-content-primary mb-1";

interface MCPServerFormProps {
	server?: TypesGen.ChatMCPServerConfig;
	isMutating: boolean;
	onSave: (
		req:
			| TypesGen.CreateChatMCPServerRequest
			| TypesGen.UpdateChatMCPServerRequest,
	) => Promise<unknown>;
	onDelete?: () => Promise<unknown>;
	onBack: () => void;
}

export const MCPServerForm: FC<MCPServerFormProps> = ({
	server,
	isMutating,
	onSave,
	onDelete,
	onBack,
}) => {
	const isEditing = !!server;
	const [slug, setSlug] = useState(server?.slug ?? "");
	const [url, setUrl] = useState(server?.url ?? "");
	const [displayName, setDisplayName] = useState(server?.display_name ?? "");
	const [authType, setAuthType] = useState<TypesGen.ChatMCPServerAuthType>(
		server?.auth_type ?? "none",
	);
	const [authHeaders, setAuthHeaders] = useState("");
	const [toolAllowRegex, setToolAllowRegex] = useState(
		server?.tool_allow_regex ?? "",
	);
	const [toolDenyRegex, setToolDenyRegex] = useState(
		server?.tool_deny_regex ?? "",
	);
	const [enabled, setEnabled] = useState(server?.enabled ?? true);
	const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();

		if (isEditing) {
			const req: TypesGen.UpdateChatMCPServerRequest = {};
			if (slug !== server.slug) (req as Record<string, unknown>).slug = slug;
			if (url !== server.url) (req as Record<string, unknown>).url = url;
			if (displayName !== server.display_name)
				(req as Record<string, unknown>).display_name = displayName;
			if (authType !== server.auth_type)
				(req as Record<string, unknown>).auth_type = authType;
			if (authHeaders) {
				try {
					(req as Record<string, unknown>).auth_headers =
						JSON.parse(authHeaders);
				} catch {
					return; // Invalid JSON, don't submit
				}
			}
			if (toolAllowRegex !== server.tool_allow_regex)
				(req as Record<string, unknown>).tool_allow_regex = toolAllowRegex;
			if (toolDenyRegex !== server.tool_deny_regex)
				(req as Record<string, unknown>).tool_deny_regex = toolDenyRegex;
			if (enabled !== server.enabled)
				(req as Record<string, unknown>).enabled = enabled;
			await onSave(req);
		} else {
			const req: TypesGen.CreateChatMCPServerRequest = {
				slug,
				url,
				display_name: displayName,
				auth_type: authType,
				tool_allow_regex: toolAllowRegex,
				tool_deny_regex: toolDenyRegex,
				enabled,
			};
			if (authHeaders) {
				try {
					(req as unknown as Record<string, unknown>).auth_headers =
						JSON.parse(authHeaders);
				} catch {
					return;
				}
			}
			await onSave(req);
		}
	};

	return (
		<>
			<SectionHeader
				label={
					<span className="flex items-center gap-2">
						<button
							type="button"
							onClick={onBack}
							className="flex items-center gap-1 border-0 bg-transparent p-0 text-content-secondary hover:text-content-primary cursor-pointer"
						>
							<ArrowLeftIcon className="h-4 w-4" />
						</button>
						{isEditing ? server.display_name || server.slug : "Add MCP Server"}
					</span>
				}
			/>
			<form className="space-y-4" onSubmit={(e) => void handleSubmit(e)}>
				<div>
					<label className={labelClassName} htmlFor="mcp-slug">
						Slug
					</label>
					<input
						id="mcp-slug"
						className={inputClassName}
						placeholder="my-server"
						value={slug}
						onChange={(e) => setSlug(e.target.value)}
						disabled={isMutating}
						required
						pattern="^[a-z0-9][a-z0-9_-]*$"
					/>
					<p className="mt-1 text-[11px] text-content-secondary">
						Lowercase letters, numbers, hyphens, and underscores.
					</p>
				</div>

				<div>
					<label className={labelClassName} htmlFor="mcp-url">
						URL
					</label>
					<input
						id="mcp-url"
						className={inputClassName}
						placeholder="https://mcp.example.com/sse"
						value={url}
						onChange={(e) => setUrl(e.target.value)}
						disabled={isMutating}
						required
						type="url"
					/>
				</div>

				<div>
					<label className={labelClassName} htmlFor="mcp-display-name">
						Display Name
					</label>
					<input
						id="mcp-display-name"
						className={inputClassName}
						placeholder="My MCP Server"
						value={displayName}
						onChange={(e) => setDisplayName(e.target.value)}
						disabled={isMutating}
					/>
				</div>

				<div>
					<label className={labelClassName} htmlFor="mcp-auth-type">
						Authentication
					</label>
					<select
						id="mcp-auth-type"
						className={inputClassName}
						value={authType}
						onChange={(e) =>
							setAuthType(e.target.value as TypesGen.ChatMCPServerAuthType)
						}
						disabled={isMutating}
					>
						<option value="none">None</option>
						<option value="header">Static Headers</option>
						<option value="oauth">OAuth (coming soon)</option>
					</select>
				</div>

				{authType === "header" && (
					<div>
						<label className={labelClassName} htmlFor="mcp-auth-headers">
							Auth Headers (JSON)
						</label>
						<textarea
							id="mcp-auth-headers"
							className={`${inputClassName} min-h-[80px] resize-y font-mono`}
							placeholder={'{"Authorization": "Bearer sk-..."}'}
							value={authHeaders}
							onChange={(e) => setAuthHeaders(e.target.value)}
							disabled={isMutating}
						/>
						{isEditing && server.has_auth_headers && !authHeaders && (
							<p className="mt-1 text-[11px] text-content-secondary">
								Headers are configured. Leave empty to keep existing headers.
							</p>
						)}
					</div>
				)}

				<div>
					<label className={labelClassName} htmlFor="mcp-tool-allow">
						Tool Allow Regex
					</label>
					<input
						id="mcp-tool-allow"
						className={inputClassName}
						placeholder=".*"
						value={toolAllowRegex}
						onChange={(e) => setToolAllowRegex(e.target.value)}
						disabled={isMutating}
					/>
					<p className="mt-1 text-[11px] text-content-secondary">
						Only tools matching this regex will be exposed. Leave empty to allow
						all.
					</p>
				</div>

				<div>
					<label className={labelClassName} htmlFor="mcp-tool-deny">
						Tool Deny Regex
					</label>
					<input
						id="mcp-tool-deny"
						className={inputClassName}
						placeholder=""
						value={toolDenyRegex}
						onChange={(e) => setToolDenyRegex(e.target.value)}
						disabled={isMutating}
					/>
					<p className="mt-1 text-[11px] text-content-secondary">
						Tools matching this regex will be hidden. Applied after allow regex.
					</p>
				</div>

				<div className="flex items-center gap-2">
					<input
						id="mcp-enabled"
						type="checkbox"
						checked={enabled}
						onChange={(e) => setEnabled(e.target.checked)}
						disabled={isMutating}
						className="h-4 w-4"
					/>
					<label
						htmlFor="mcp-enabled"
						className="text-[13px] text-content-primary"
					>
						Enabled
					</label>
				</div>

				<div className="flex items-center justify-between pt-2">
					<div>
						{isEditing && onDelete && (
							<>
								{showDeleteConfirm ? (
									<div className="flex items-center gap-2">
										<span className="text-[12px] text-content-destructive">
											Delete this server?
										</span>
										<Button
											size="sm"
											variant="destructive"
											type="button"
											disabled={isMutating}
											onClick={() => void onDelete()}
										>
											Confirm
										</Button>
										<Button
											size="sm"
											variant="outline"
											type="button"
											onClick={() => setShowDeleteConfirm(false)}
										>
											Cancel
										</Button>
									</div>
								) : (
									<Button
										size="sm"
										variant="outline"
										type="button"
										onClick={() => setShowDeleteConfirm(true)}
										disabled={isMutating}
									>
										<Trash2Icon className="h-4 w-4" />
										Delete
									</Button>
								)}
							</>
						)}
					</div>
					<Button
						size="sm"
						type="submit"
						disabled={isMutating || !slug || !url}
					>
						{isEditing ? "Save" : "Create"}
					</Button>
				</div>
			</form>
		</>
	);
};
