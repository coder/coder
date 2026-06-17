/**
 * Returns the English indefinite article ("a" or "an") for the given word,
 * based on whether it starts with a vowel.
 */
export const indefiniteArticle = (word: string): string =>
	/^[aeiou]/i.test(word) ? "an" : "a";
