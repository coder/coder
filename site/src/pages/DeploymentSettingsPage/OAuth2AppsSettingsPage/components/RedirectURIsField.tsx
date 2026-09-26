import { PlusIcon, XIcon } from "lucide-react";
import { type FC, useId } from "react";
import { OAuth2RedirectURIsMaxCount } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import type { FormHelpers } from "#/utils/formUtils";

type RedirectURIsFieldProps = {
	values: string[];
	onChange: (values: string[]) => void;
	onBlur: () => void;
	disabled: boolean;
	isPublicClient: boolean;
	errors?: string[];
};

export const RedirectURIsField: FC<RedirectURIsFieldProps> = ({
	values,
	onChange,
	onBlur,
	disabled,
	isPublicClient,
	errors,
}) => {
	const baseId = useId();
	const atMax = values.length >= OAuth2RedirectURIsMaxCount;

	const setEntry = (index: number, value: string) => {
		onChange(values.map((v, i) => (i === index ? value : v)));
	};

	const removeEntry = (index: number) => {
		onChange(values.filter((_, i) => i !== index));
	};

	return (
		<div className="flex flex-col gap-3">
			<div className="flex flex-col gap-1">
				<span className="text-sm font-medium">Redirect URIs</span>
				<span className="text-xs text-content-secondary">
					{isPublicClient
						? "URIs this application may redirect to. This app has no client secret, so http is limited to a loopback address; https, a custom scheme, and the out-of-band URN are also accepted."
						: "URIs this application may redirect to after a user authorizes it."}
				</span>
			</div>

			{values.map((value, index) => {
				const fieldHelpers: FormHelpers = {
					name: `redirect_uris.${index}`,
					id: `${baseId}-${index}`,
					value,
					error: Boolean(errors?.[index]),
					helperText: errors?.[index],
					onChange: (event) => setEntry(index, event.target.value),
					onBlur,
				};

				return (
					<div key={index} className="flex items-start gap-2">
						<div className="flex-1">
							<FormField
								field={fieldHelpers}
								label={
									index === 0 ? "Default callback" : `Redirect URI ${index + 1}`
								}
								disabled={disabled}
								required={index === 0}
							/>
						</div>
						<Button
							type="button"
							variant="outline"
							size="icon"
							disabled={disabled}
							aria-label={`Remove redirect URI ${index + 1}`}
							className="mt-8"
							onClick={() => removeEntry(index)}
						>
							<XIcon />
						</Button>
					</div>
				);
			})}

			<Button
				type="button"
				variant="outline"
				disabled={disabled || atMax}
				className="self-start"
				onClick={() => onChange([...values, ""])}
			>
				<PlusIcon />
				Add redirect URI
			</Button>
		</div>
	);
};
