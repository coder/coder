"use strict";

/**
 * Converts a title into a lowercase, dash-separated URL slug.
 * @param {string} text
 * @param {{maxLength?: number}} [options] maxLength cuts the slug at the
 *   last word boundary that fits, or mid-word when the first word is longer.
 * @returns {string}
 */
function slugify(text, options = {}) {
  let slug = String(text)
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  const max = options.maxLength;
  if (max !== undefined && slug.length > max) {
    const cut = slug.slice(0, max + 1);
    const boundary = cut.lastIndexOf("-");
    slug = boundary > 0 ? cut.slice(0, boundary) : slug.slice(0, max);
  }
  return slug;
}

module.exports = { slugify };
