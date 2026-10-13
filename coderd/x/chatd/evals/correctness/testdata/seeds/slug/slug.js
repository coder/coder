"use strict";

/**
 * Converts a title into a lowercase, dash-separated URL slug.
 * @param {string} text
 * @returns {string}
 */
function slugify(text) {
  return String(text)
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

module.exports = { slugify };
