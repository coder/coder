import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { Alert, AlertDescription } from "./Alert";

type InvisibleCharactersAlertProps = {
	readonly text: string;
	readonly className?: string;
};

/**
 * Warns about invisible Unicode in prompt text that the server strips on
 * save. Renders nothing when the text has none.
 */
export const InvisibleCharactersAlert: React.FC<
	InvisibleCharactersAlertProps
> = ({ text, className }) => {
	const count = countInvisibleCharacters(text);
	if (count === 0) {
		return null;
	}

	return (
		<Alert severity="warning" className={className}>
			<AlertDescription>
				This text contains {count} invisible Unicode{" "}
				{count !== 1 ? "characters" : "character"} that could hide content.
				These will be stripped on save.
			</AlertDescription>
		</Alert>
	);
};
