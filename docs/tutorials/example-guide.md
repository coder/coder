# Guide Title (Only Visible in GitHub)

<div>
  <a href="https://github.com/coder" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Your Name</span>
    <img src="https://github.com/coder.png" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
December 13, 2023

---

This is a guide on how to make Coder guides, it is not listed on our
[official tutorials page](../tutorials/index.md) in the docs. Intended for those
who don't frequently contribute documentation changes to the `coder/coder`
repository.

## Content

Defer to our [Contributing/Documentation](../contributing/documentation.md) page
for rules on technical writing.

### Adding Photos

Use relative imports in the markdown and store photos in
`docs/images/guides/<your_guide>/<image>.png`.

### Setting the author data

At the top of this example you will find a small html snippet that nicely
renders the author's name and photo, while linking to their GitHub profile.
Before submitting your guide in a PR, replace `your_github_handle`,
`your_github_profile_photo_url` and "Your Name". The entire `<img>` element can
be omitted.

## Setting up the routes

Once you've written your guide, you'll need to add its route to
`docs/manifest.json` under `Guides` > `"children"` at the bottom:

```json
{
  // Overrides the "# Guide Title" at the top of this file
  "title": "Contributing to Guides",
  "description": "How to add a guide",
  "path": "./guides/my-guide-file.md"
},
```

## Format before push

Before pushing your guide to github, run `make fmt` to format the files with
Prettier. Then, push your changes to a new branch and create a PR.
