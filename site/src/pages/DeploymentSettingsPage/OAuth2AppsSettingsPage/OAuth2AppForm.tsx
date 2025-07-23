import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import IconButton from "@mui/material/IconButton";
import TextField from "@mui/material/TextField";
import { isApiValidationError, mapApiErrorToFieldErrors } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import type { FC, ReactNode } from "react";
import { useState } from "react";

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: {
		name: string;
		redirect_uris: string[];
		icon: string;
	}) => void;
	error?: unknown;
	isUpdating: boolean;
	actions?: ReactNode;
};

export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
	app,
	onSubmit,
	error,
	isUpdating,
	actions,
}) => {
	// Initialize redirect URIs state with existing app data or a single empty field
	const [redirectUris, setRedirectUris] = useState<string[]>(() => {
		if (app?.redirect_uris && app.redirect_uris.length > 0) {
			return [...app.redirect_uris];
		}
		return [""];
	});

	const apiValidationErrors = isApiValidationError(error)
		? mapApiErrorToFieldErrors(error.response.data)
		: undefined;

	const addRedirectUri = () => {
		setRedirectUris([...redirectUris, ""]);
	};

	const removeRedirectUri = (index: number) => {
		if (redirectUris.length > 1) {
			setRedirectUris(redirectUris.filter((_, i) => i !== index));
		}
	};

	const updateRedirectUri = (index: number, value: string) => {
		const updated = [...redirectUris];
		updated[index] = value;
		setRedirectUris(updated);
	};

	return (
		<form
			css={{ marginTop: 10 }}
			onSubmit={(event) => {
				event.preventDefault();
				const formData = new FormData(event.target as HTMLFormElement);

				// Filter out empty redirect URIs and validate we have at least one
				const filteredRedirectUris = redirectUris.filter(
					(uri) => uri.trim() !== "",
				);

				onSubmit({
					name: formData.get("name") as string,
					redirect_uris: filteredRedirectUris,
					icon: formData.get("icon") as string,
				});
			}}
		>
			<Stack spacing={2.5}>
				<TextField
					name="name"
					label="Application name"
					defaultValue={app?.name}
					error={Boolean(apiValidationErrors?.name)}
					helperText={
						apiValidationErrors?.name || "The name of your Coder app."
					}
					autoFocus
					fullWidth
				/>

				{/* Redirect URIs Section */}
				<Stack spacing={1}>
					<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
						<span css={{ fontWeight: 500, fontSize: 14 }}>Redirect URIs</span>
						<IconButton
							size="small"
							onClick={addRedirectUri}
							css={{ padding: 4 }}
							title="Add redirect URI"
						>
							<AddIcon fontSize="small" />
						</IconButton>
					</div>

					{redirectUris.map((uri, index) => (
						<Stack
							key={index}
							direction="row"
							spacing={1}
							sx={{ alignItems: "flex-start" }}
						>
							<TextField
								value={uri}
								onChange={(event) =>
									updateRedirectUri(index, event.target.value)
								}
								placeholder="https://example.com/callback"
								error={Boolean(apiValidationErrors?.redirect_uris)}
								helperText={
									index === 0 &&
									(apiValidationErrors?.redirect_uris ||
										"Full URLs where users will be redirected after authorization. At least one is required.")
								}
								fullWidth
							/>
							{redirectUris.length > 1 && (
								<Stack sx={{ paddingTop: 1 }}>
									<IconButton
										size="small"
										onClick={() => removeRedirectUri(index)}
										css={{ padding: 4 }}
										title="Remove redirect URI"
									>
										<DeleteIcon fontSize="small" />
									</IconButton>
								</Stack>
							)}
						</Stack>
					))}
				</Stack>

				<TextField
					name="icon"
					label="Application icon"
					defaultValue={app?.icon}
					error={Boolean(apiValidationErrors?.icon)}
					helperText={
						apiValidationErrors?.icon || "A full or relative URL to an icon."
					}
					fullWidth
				/>

				<Stack direction="row">
					<Button disabled={isUpdating} type="submit">
						<Spinner loading={isUpdating} />
						{app ? "Update application" : "Create application"}
					</Button>
					{actions}
				</Stack>
			</Stack>
		</form>
	);
};
