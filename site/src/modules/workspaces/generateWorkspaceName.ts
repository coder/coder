import isChromatic from "chromatic/isChromatic";
import {
	animals,
	colors,
	NumberDictionary,
	uniqueNamesGenerator,
} from "unique-names-generator";

export const generateWorkspaceName = () => {
	if (isChromatic()) {
		return "chromatic-workspace";
	}
	const numberDictionary = NumberDictionary.generate({ min: 0, max: 99 });
	return uniqueNamesGenerator({
		dictionaries: [colors, animals, numberDictionary],
		separator: "-",
		length: 3,
		style: "lowerCase",
	});
};
