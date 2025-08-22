import isChromatic from "chromatic/isChromatic";
import {
	animals,
	colors,
	NumberDictionary,
	uniqueNamesGenerator,
} from "unique-names-generator";

export const generateWorkspaceName = () => {
	if (isChromatic()) {
		return "yellow-bird-23";
	}
	const numberDictionary = NumberDictionary.generate({ min: 0, max: 99 });
	return uniqueNamesGenerator({
		dictionaries: [colors, animals, numberDictionary],
		separator: "-",
		length: 3,
		style: "lowerCase",
	});
};
