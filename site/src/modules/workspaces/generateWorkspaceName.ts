import {
  NumberDictionary,
  animals,
  colors,
  uniqueNamesGenerator,
} from "unique-names-generator";

export const generateWorkspaceName = () => {
  const numberDictionary = NumberDictionary.generate({ min: 0, max: 99 });
  return uniqueNamesGenerator({
    dictionaries: [colors, animals, numberDictionary],
    separator: "-",
    length: 3,
    style: "lowerCase",
  });
};
